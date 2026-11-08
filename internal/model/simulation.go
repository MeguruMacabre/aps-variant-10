package model

import (
	"container/heap"
	"context"
	"fmt"
	"math/rand"
	"sort"
)

type Status string

const (
	Generated  Status = "создана"
	Waiting    Status = "в буфере"
	Processing Status = "обслуживается"
	Completed  Status = "обслужена"
	Refused    Status = "отказ"
)

type Request struct {
	ID                                            string
	UID                                           int64
	SourceID                                      int
	GeneratedAt, StartedAt, FinishedAt, RefusedAt float64
	Status                                        Status
}

type Source struct {
	Config                                             SourceConfig
	Generated, Completed, Refused, Waiting, Processing int
	rng                                                *rand.Rand
}

type Device struct {
	Config             DeviceConfig
	Current            *Request
	FinishAt, BusyTime float64
	Completed          int
	rng                *rand.Rand
}

type Buffer struct{ Slots []*Request }

func (b *Buffer) FirstFree() int {
	for i, r := range b.Slots {
		if r == nil {
			return i
		}
	}
	return -1
}
func (b *Buffer) Empty() bool {
	for _, r := range b.Slots {
		if r != nil {
			return false
		}
	}
	return true
}
func (b *Buffer) LowestPriorityOldest() int {
	v := -1
	for i, r := range b.Slots {
		if r == nil {
			continue
		}
		if v < 0 || r.SourceID > b.Slots[v].SourceID || r.SourceID == b.Slots[v].SourceID && older(r, b.Slots[v]) {
			v = i
		}
	}
	return v
}
func older(a, b *Request) bool {
	return a.GeneratedAt < b.GeneratedAt || a.GeneratedAt == b.GeneratedAt && a.UID < b.UID
}

type EventKind int

const (
	ServiceCompletion EventKind = iota
	Arrival
)

type Event struct {
	Time   float64
	Kind   EventKind
	Index  int
	Serial uint64
}

func (e Event) Label() string {
	if e.Kind == Arrival {
		return fmt.Sprintf("Поступление И%d", e.Index+1)
	}
	return fmt.Sprintf("Завершение П%d", e.Index+1)
}
func eventLess(a, b Event) bool {
	if a.Time != b.Time {
		return a.Time < b.Time
	}
	if a.Kind != b.Kind {
		return a.Kind < b.Kind
	}
	if a.Index != b.Index {
		return a.Index < b.Index
	}
	return a.Serial < b.Serial
}

type EventCalendar []Event

func (c EventCalendar) Len() int           { return len(c) }
func (c EventCalendar) Less(i, j int) bool { return eventLess(c[i], c[j]) }
func (c EventCalendar) Swap(i, j int)      { c[i], c[j] = c[j], c[i] }
func (c *EventCalendar) Push(x any)        { *c = append(*c, x.(Event)) }
func (c *EventCalendar) Pop() any          { a := *c; e := a[len(a)-1]; *c = a[:len(a)-1]; return e }

type Packet struct {
	SourceID  int
	Remaining map[int64]bool
}
type PlacementDispatcher struct{}
type SelectionDispatcher struct {
	NextDevice   int
	ActivePacket *Packet
}

func (v *SelectionDispatcher) FindFreeDevice(m *Simulation) int {
	for offset := range m.Devices {
		i := (v.NextDevice + offset) % len(m.Devices)
		if m.Devices[i].Current == nil {
			return i
		}
	}
	return -1
}
func (v *SelectionDispatcher) FormPacket(m *Simulation) {
	source := -1
	for _, r := range m.Buffer.Slots {
		if r != nil && (source < 0 || r.SourceID < source) {
			source = r.SourceID
		}
	}
	p := &Packet{SourceID: source, Remaining: make(map[int64]bool)}
	for _, r := range m.Buffer.Slots {
		if r != nil && r.SourceID == source {
			p.Remaining[r.UID] = true
		}
	}
	v.ActivePacket = p
	if source >= 0 {
		m.note("Новый пакет И%d: %d заявок", source+1, len(p.Remaining))
	}
}
func (v *SelectionDispatcher) SelectRequest(m *Simulation) *Request {
	if v.ActivePacket == nil || len(v.ActivePacket.Remaining) == 0 {
		v.FormPacket(m)
	}
	best := -1
	for i, r := range m.Buffer.Slots {
		if r != nil && v.ActivePacket.Remaining[r.UID] && (best < 0 || older(m.Buffer.Slots[best], r)) {
			best = i
		}
	}
	if best < 0 {
		return nil
	}
	r := m.Buffer.Slots[best]
	m.Buffer.Slots[best] = nil
	m.Sources[r.SourceID].Waiting--
	delete(v.ActivePacket.Remaining, r.UID)
	m.note("Заявка %s извлечена из Б%d", r.ID, best+1)
	return r
}
func (v *SelectionDispatcher) Dispatch(m *Simulation) error {
	for !m.Buffer.Empty() {
		device := v.FindFreeDevice(m)
		if device < 0 {
			return nil
		}
		r := v.SelectRequest(m)
		if r == nil {
			return fmt.Errorf("внутренняя ошибка: непустой буфер без заявки пакета")
		}
		if err := m.start(r, device); err != nil {
			return err
		}
	}
	return nil
}
func (p *PlacementDispatcher) Accept(m *Simulation, r *Request) error {
	if m.Buffer.Empty() {
		if d := m.Selection.FindFreeDevice(m); d >= 0 {
			return m.start(r, d)
		}
	}
	slot := m.Buffer.FirstFree()
	if slot < 0 {
		slot = m.Buffer.LowestPriorityOldest()
		if slot < 0 {
			m.refuse(r)
			m.note("Нет буфера: отказ новой заявке %s", r.ID)
			return nil
		}
		victim := m.Buffer.Slots[slot]
		m.refuse(victim)
		m.Sources[victim.SourceID].Waiting--
		if m.Selection.ActivePacket != nil {
			delete(m.Selection.ActivePacket.Remaining, victim.UID)
		}
		m.note("Отказ %s; новая %s занимает Б%d", victim.ID, r.ID, slot+1)
	}
	m.Buffer.Slots[slot] = r
	r.Status = Waiting
	m.Sources[r.SourceID].Waiting++
	m.note("Заявка %s записана в Б%d без сдвига", r.ID, slot+1)
	return nil
}

type Simulation struct {
	Config           Config
	Now              float64
	Sources          []Source
	Devices          []Device
	Buffer           Buffer
	Calendar         EventCalendar
	Placement        PlacementDispatcher
	Selection        SelectionDispatcher
	Statistics       []SourceStatistics
	Generated, Steps int
	Last             *Event
	Changes          []string
	Detailed         bool
	Demo             bool
	serial           uint64
	durations        map[string]float64
}

func New(c Config) (*Simulation, error) { return newSimulation(c, false) }
func newSimulation(c Config, demo bool) (*Simulation, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	m := &Simulation{Config: c, Sources: make([]Source, len(c.Sources)), Devices: make([]Device, len(c.Devices)), Statistics: make([]SourceStatistics, len(c.Sources)), Buffer: Buffer{make([]*Request, c.BufferSize)}, Demo: demo}
	for i, s := range c.Sources {
		m.Sources[i] = Source{Config: s, rng: rand.New(rand.NewSource(c.Seed + int64(i+1)*7919))}
	}
	for i, d := range c.Devices {
		m.Devices[i] = Device{Config: d, rng: rand.New(rand.NewSource(c.Seed + int64(i+1)*104729 + 10000019))}
	}
	if !demo {
		for i := range m.Sources {
			if err := m.nextArrival(i); err != nil {
				return nil, err
			}
		}
	}
	return m, nil
}

func NewDemo() (*Simulation, error) {
	c := DefaultConfig()
	c.Requests = 8
	m, err := newSimulation(c, true)
	if err != nil {
		return nil, err
	}
	for _, a := range []struct {
		time   float64
		source int
	}{{0, 2}, {1, 1}, {2, 2}, {3, 1}, {4, 1}, {5, 0}, {7, 0}, {8, 1}} {
		m.push(Event{Time: a.time, Kind: Arrival, Index: a.source})
	}
	m.durations = map[string]float64{"3.1": 6, "2.1": 5, "2.2": 3, "2.3": 3, "1.1": 4, "1.2": 3, "2.4": 2}
	return m, nil
}

func (m *Simulation) note(format string, args ...any) {
	if m.Detailed {
		m.Changes = append(m.Changes, fmt.Sprintf(format, args...))
	}
}
func (m *Simulation) push(e Event) { m.serial++; e.Serial = m.serial; heap.Push(&m.Calendar, e) }
func (m *Simulation) nextArrival(i int) error {
	s := &m.Sources[i]
	delta := s.Config.Min + s.rng.Float64()*(s.Config.Max-s.Config.Min)
	time := m.Now + delta
	if !finite(time) || time <= m.Now {
		return fmt.Errorf("И%d: интервал нельзя представить при текущем масштабе модельного времени", i+1)
	}
	m.push(Event{Time: time, Kind: Arrival, Index: i})
	return nil
}
func (m *Simulation) cancelArrivals() {
	kept := m.Calendar[:0]
	for _, e := range m.Calendar {
		if e.Kind != Arrival {
			kept = append(kept, e)
		}
	}
	m.Calendar = kept
	heap.Init(&m.Calendar)
}
func (m *Simulation) refuse(r *Request) {
	r.Status = Refused
	r.RefusedAt = m.Now
	m.Sources[r.SourceID].Refused++
}
func (m *Simulation) start(r *Request, i int) error {
	d := &m.Devices[i]
	var duration float64
	if m.Demo {
		var ok bool
		duration, ok = m.durations[r.ID]
		if !ok {
			return fmt.Errorf("нет демонстрационной длительности для %s", r.ID)
		}
	} else {
		for duration == 0 {
			duration = d.rng.ExpFloat64() / d.Config.Rate
		}
	}
	finish := m.Now + duration
	if !finite(duration) || duration <= 0 || !finite(finish) || finish <= m.Now {
		return fmt.Errorf("П%d: время обслуживания нельзя представить; проверьте масштаб параметров", i+1)
	}
	r.StartedAt = m.Now
	r.Status = Processing
	d.Current = r
	d.FinishAt = finish
	m.Sources[r.SourceID].Processing++
	m.Selection.NextDevice = (i + 1) % len(m.Devices)
	m.push(Event{Time: finish, Kind: ServiceCompletion, Index: i})
	m.note("Начало %s на П%d; ожидание %.6g; завершение %.6g", r.ID, i+1, m.Now-r.GeneratedAt, finish)
	return nil
}
func (m *Simulation) Done() bool { return len(m.Calendar) == 0 }
func (m *Simulation) Step() (bool, error) {
	if m.Done() {
		return false, nil
	}
	if p := m.Selection.ActivePacket; p != nil && len(p.Remaining) == 0 && m.Buffer.Empty() {
		m.Selection.ActivePacket = nil
	}
	m.Changes = nil
	e := heap.Pop(&m.Calendar).(Event)
	m.Now = e.Time
	m.Last = &e
	m.Steps++
	if e.Kind == ServiceCompletion {
		d := &m.Devices[e.Index]
		r := d.Current
		if r == nil {
			return false, fmt.Errorf("событие завершения на свободном П%d", e.Index+1)
		}
		r.FinishedAt = m.Now
		r.Status = Completed
		d.BusyTime += m.Now - r.StartedAt
		d.Completed++
		d.Current = nil
		d.FinishAt = 0
		s := &m.Sources[r.SourceID]
		s.Processing--
		s.Completed++
		m.Statistics[r.SourceID].Wait.Add(r.StartedAt - r.GeneratedAt)
		m.Statistics[r.SourceID].Service.Add(r.FinishedAt - r.StartedAt)
		m.Statistics[r.SourceID].Residence.Add(r.FinishedAt - r.GeneratedAt)
		m.note("Заявка %s обслужена, П%d освобождён", r.ID, e.Index+1)
	} else {
		if err := m.Selection.Dispatch(m); err != nil {
			return false, err
		}
		s := &m.Sources[e.Index]
		s.Generated++
		m.Generated++
		r := &Request{ID: fmt.Sprintf("%d.%d", e.Index+1, s.Generated), UID: int64(m.Generated), SourceID: e.Index, GeneratedAt: m.Now, Status: Generated}
		m.note("Поступила заявка %s", r.ID)
		if m.Generated >= m.Config.Requests {
			m.cancelArrivals()
		} else if !m.Demo {
			if err := m.nextArrival(e.Index); err != nil {
				return false, err
			}
		}
		if err := m.Placement.Accept(m, r); err != nil {
			return false, err
		}
	}
	if err := m.Selection.Dispatch(m); err != nil {
		return false, err
	}
	return true, nil
}

func (m *Simulation) Run(ctx context.Context) error {
	for !m.Done() {
		if m.Steps%1024 == 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
		}
		if _, err := m.Step(); err != nil {
			return err
		}
	}
	return nil
}
func (m *Simulation) Events() []Event {
	a := append([]Event(nil), m.Calendar...)
	sort.Slice(a, func(i, j int) bool { return eventLess(a[i], a[j]) })
	return a
}
func (m *Simulation) PacketIDs() []string {
	var a []*Request
	if m.Selection.ActivePacket != nil {
		for _, r := range m.Buffer.Slots {
			if r != nil && m.Selection.ActivePacket.Remaining[r.UID] {
				a = append(a, r)
			}
		}
	}
	sort.Slice(a, func(i, j int) bool { return older(a[i], a[j]) })
	ids := make([]string, 0, len(a))
	for _, r := range a {
		ids = append(ids, r.ID)
	}
	return ids
}
