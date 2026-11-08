package model

import (
	"context"
	"encoding/csv"
	"fmt"
	"math"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func checkInvariant(t *testing.T, m *Simulation) {
	t.Helper()
	seen := map[int64]bool{}
	waiting := make([]int, len(m.Sources))
	processing := make([]int, len(m.Sources))
	for _, r := range m.Buffer.Slots {
		if r == nil {
			continue
		}
		if seen[r.UID] || r.Status != Waiting {
			t.Fatal("duplicate/wrong buffered request", r)
		}
		seen[r.UID] = true
		waiting[r.SourceID]++
	}
	free := false
	for _, d := range m.Devices {
		r := d.Current
		if r == nil {
			free = true
			continue
		}
		if seen[r.UID] || r.Status != Processing || d.FinishAt < m.Now {
			t.Fatal("duplicate/wrong processing request", r)
		}
		seen[r.UID] = true
		processing[r.SourceID]++
	}
	if free && !m.Buffer.Empty() {
		t.Fatal("free device with waiting work")
	}
	generated := 0
	for i, s := range m.Sources {
		generated += s.Generated
		if s.Generated != s.Completed+s.Refused+s.Waiting+s.Processing || waiting[i] != s.Waiting || processing[i] != s.Processing {
			t.Fatalf("source balance %d: %+v", i, s)
		}
	}
	if generated != m.Generated {
		t.Fatal("global generated balance")
	}
	if p := m.Selection.ActivePacket; p != nil {
		for id := range p.Remaining {
			found := false
			for _, r := range m.Buffer.Slots {
				if r != nil && r.UID == id && r.SourceID == p.SourceID {
					found = true
				}
			}
			if !found {
				t.Fatal("packet contains nonbuffered ID", id)
			}
		}
	}
	for _, e := range m.Calendar {
		if e.Time < m.Now {
			t.Fatal("event in past")
		}
	}
	for _, d := range m.Report().Devices {
		if d.Utilization < 0 || d.Utilization > 1+1e-10 {
			t.Fatal("invalid utilization", d)
		}
	}
}

func TestDemoMatchesExistingTimeline(t *testing.T) {
	b, err := os.ReadFile("../../docs/timeline-events.csv")
	if err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(b), "\ufeff"))).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	m, err := NewDemo()
	if err != nil {
		t.Fatal(err)
	}
	for index, row := range rows[1:] {
		ok, err := m.Step()
		if err != nil || !ok {
			t.Fatalf("step %d: %v", index+1, err)
		}
		checkInvariant(t, m)
		if fmt.Sprint(m.Now) != row[0] {
			t.Fatalf("step %d time %g != %s", index+1, m.Now, row[0])
		}
		for i, r := range m.Buffer.Slots {
			id := "—"
			if r != nil {
				id = r.ID
			}
			if id != row[i+2] {
				t.Fatalf("step %d buffer %d: %s != %s", index+1, i+1, id, row[i+2])
			}
		}
		for i, d := range m.Devices {
			s := "свободен"
			if d.Current != nil {
				s = fmt.Sprintf("%s до %g", d.Current.ID, d.FinishAt)
			}
			if s != row[i+5] {
				t.Fatalf("step %d device %d: %s != %s", index+1, i+1, s, row[i+5])
			}
		}
		packet := "—"
		if p := m.Selection.ActivePacket; p != nil {
			packet = fmt.Sprintf("И%d: пуст", p.SourceID+1)
			if ids := m.PacketIDs(); len(ids) > 0 {
				packet = fmt.Sprintf("И%d: {%s}", p.SourceID+1, strings.Join(ids, ", "))
			}
		}
		if packet != row[7] || strconv.Itoa(m.Selection.NextDevice+1) != row[8] {
			t.Fatalf("step %d packet/pointer: %s / %d; expected %s / %s", index+1, packet, m.Selection.NextDevice+1, row[7], row[8])
		}
	}
	r := m.Report()
	if !m.Done() || r.Generated != 8 || r.Completed != 7 || r.Refused != 1 || r.Time != 14 {
		t.Fatalf("wrong demo summary: %+v", r)
	}
	if r.Sources[0].MeanWait != 2 || r.Sources[0].MeanService != 3.5 || r.Sources[0].MeanResidence != 5.5 {
		t.Fatal("incorrect source 1 times", r.Sources[0])
	}
	if r.Sources[0].VarianceWait != 2 || r.Sources[0].VarianceService != .5 {
		t.Fatal("incorrect sample variances", r.Sources[0])
	}
	if math.Abs(r.Devices[0].Utilization-13./14) > 1e-12 || math.Abs(r.Devices[1].Utilization-13./14) > 1e-12 {
		t.Fatal("incorrect utilization", r.Devices)
	}
}

func TestBufferAndEviction(t *testing.T) {
	b := Buffer{Slots: []*Request{{UID: 1, SourceID: 0, GeneratedAt: 1}, nil, {UID: 3, SourceID: 2, GeneratedAt: 2}}}
	if b.FirstFree() != 1 {
		t.Fatal("first free slot")
	}
	b.Slots[b.FirstFree()] = &Request{UID: 2, SourceID: 2, GeneratedAt: 1}
	if b.LowestPriorityOldest() != 1 {
		t.Fatal("priority then oldest")
	}
	b.Slots[1].GeneratedAt = 2
	if b.LowestPriorityOldest() != 1 {
		t.Fatal("ID tie-break")
	}
	c := DefaultConfig()
	m, _ := New(c)
	m.Calendar = nil
	m.Now = 5
	for i := range m.Devices {
		m.Devices[i].Current = &Request{ID: "busy"}
	}
	m.Buffer.Slots = []*Request{{ID: "1.1", UID: 1, SourceID: 0, GeneratedAt: 1, Status: Waiting}, {ID: "1.2", UID: 2, SourceID: 0, GeneratedAt: 2, Status: Waiting}, {ID: "2.1", UID: 3, SourceID: 1, GeneratedAt: 3, Status: Waiting}}
	m.Sources[0].Waiting = 2
	m.Sources[1].Waiting = 1
	m.Selection.ActivePacket = &Packet{SourceID: 1, Remaining: map[int64]bool{3: true}}
	r := &Request{ID: "3.1", UID: 4, SourceID: 2, GeneratedAt: 5}
	if err := m.Placement.Accept(m, r); err != nil {
		t.Fatal(err)
	}
	if m.Buffer.Slots[2] != r || m.Sources[1].Refused != 1 || len(m.Selection.ActivePacket.Remaining) != 0 {
		t.Fatal("new lower priority request must replace old buffered request, removing its packet ID")
	}
	if m.Buffer.Slots[0].ID != "1.1" || m.Buffer.Slots[1].ID != "1.2" {
		t.Fatal("buffer shifted")
	}
}

func TestCalendarTieOrder(t *testing.T) {
	a := []Event{{Time: 1, Kind: Arrival, Index: 0}, {Time: 1, Kind: ServiceCompletion, Index: 1}, {Time: 1, Kind: ServiceCompletion, Index: 0}, {Time: 1, Kind: Arrival, Index: 1}}
	m := &Simulation{}
	for _, e := range a {
		m.push(e)
	}
	want := []Event{{Time: 1, Kind: ServiceCompletion, Index: 0}, {Time: 1, Kind: ServiceCompletion, Index: 1}, {Time: 1, Kind: Arrival, Index: 0}, {Time: 1, Kind: Arrival, Index: 1}}
	for i, e := range m.Events() {
		if e.Kind != want[i].Kind || e.Index != want[i].Index {
			t.Fatal("tie order", m.Events())
		}
	}
}

func TestRandomInvariantsAndDrain(t *testing.T) {
	for seed := int64(1); seed <= 12; seed++ {
		for _, size := range []int{0, 1, 3, 8} {
			c := DefaultConfig()
			c.Seed = seed
			c.Requests = 600
			c.BufferSize = size
			m, err := New(c)
			if err != nil {
				t.Fatal(err)
			}
			for !m.Done() {
				if _, err = m.Step(); err != nil {
					t.Fatal(err)
				}
				checkInvariant(t, m)
			}
			r := m.Report()
			if r.Generated != c.Requests || r.Completed+r.Refused != r.Generated {
				t.Fatal("did not drain", r)
			}
			for _, s := range r.Sources {
				if math.Abs(s.MeanResidence-s.MeanService-s.MeanWait) > 1e-8 {
					t.Fatal("residence != service + wait")
				}
			}
		}
	}
	c := DefaultConfig()
	c.Requests = 1
	m, _ := New(c)
	if err := m.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if m.Steps != 2 || m.Report().Completed != 1 {
		t.Fatal("pending arrivals must be cancelled at cap")
	}
}

func TestSeedReproducibility(t *testing.T) {
	c := DefaultConfig()
	c.Requests = 1000
	a, _ := New(c)
	b, _ := New(c)
	if err := a.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	for !b.Done() {
		if _, err := b.Step(); err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(a.Report(), b.Report()) {
		t.Fatal("run and stepping differ")
	}
}

func TestPrecisionHandlesZeroRefusals(t *testing.T) {
	c := DefaultConfig()
	c.Sources = []SourceConfig{{"one", 10, 11}}
	c.Devices = []DeviceConfig{{"fast", 1e6}}
	c.Requests = 10
	c.Precision.MaxRequests = 40
	x, err := RunAutomatic(context.Background(), c, nil)
	if err != nil {
		t.Fatal(err)
	}
	if x.Precision.Met || x.Summary.Generated != 40 || len(x.Precision.Iterations) != 3 {
		t.Fatalf("zero refusals cannot confirm relative precision: %+v", x.Precision)
	}
	if !math.IsInf(RequiredCount(0, .1, .9), 1) {
		t.Fatal("undefined precision for p=0")
	}
	z := math.Sqrt2 * math.Erfinv(.9)
	want := math.Ceil(z * z * .8 / (.2 * .01))
	if RequiredCount(.2, .1, .9) != want {
		t.Fatal("incorrect sample-size formula")
	}
}

func TestInvalidConfig(t *testing.T) {
	c := DefaultConfig()
	c.Sources[0].Max = math.Inf(1)
	if _, err := New(c); err == nil {
		t.Fatal("accepted infinity")
	}
	c = DefaultConfig()
	c.Devices[0].Rate = 0
	if _, err := New(c); err == nil {
		t.Fatal("accepted zero rate")
	}
	c = DefaultConfig()
	c.Sources[0].Min = c.Sources[0].Max
	if _, err := New(c); err == nil {
		t.Fatal("accepted invalid interval")
	}
}
