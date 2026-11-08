package model

type Moments struct {
	Count    int
	Mean, M2 float64
}

func (m *Moments) Add(x float64) {
	m.Count++
	delta := x - m.Mean
	m.Mean += delta / float64(m.Count)
	m.M2 += delta * (x - m.Mean)
}
func (m Moments) Variance() float64 {
	if m.Count < 2 {
		return 0
	}
	return m.M2 / float64(m.Count-1)
}

type SourceStatistics struct{ Wait, Service, Residence Moments }

type SourceResult struct {
	ID                 int     `json:"id"`
	Name               string  `json:"name"`
	Generated          int     `json:"generated"`
	Completed          int     `json:"completed"`
	Refused            int     `json:"refused"`
	Waiting            int     `json:"waiting"`
	Processing         int     `json:"processing"`
	RefusalProbability float64 `json:"refusal_probability"`
	MeanResidence      float64 `json:"mean_residence"`
	MeanWait           float64 `json:"mean_wait"`
	MeanService        float64 `json:"mean_service"`
	VarianceWait       float64 `json:"variance_wait"`
	VarianceService    float64 `json:"variance_service"`
	VarianceDefined    bool    `json:"variance_defined"`
}
type DeviceResult struct {
	ID          int     `json:"id"`
	Name        string  `json:"name"`
	Completed   int     `json:"completed"`
	BusyTime    float64 `json:"busy_time"`
	Utilization float64 `json:"utilization"`
}
type Report struct {
	Finished           bool           `json:"finished"`
	Demo               bool           `json:"demo"`
	Time               float64        `json:"simulation_time"`
	Steps              int            `json:"events"`
	Generated          int            `json:"generated"`
	Completed          int            `json:"completed"`
	Refused            int            `json:"refused"`
	RefusalProbability float64        `json:"refusal_probability"`
	Sources            []SourceResult `json:"sources"`
	Devices            []DeviceResult `json:"devices"`
}

func (m *Simulation) Report() Report {
	r := Report{Finished: m.Done(), Demo: m.Demo, Time: m.Now, Steps: m.Steps, Generated: m.Generated, Sources: make([]SourceResult, len(m.Sources)), Devices: make([]DeviceResult, len(m.Devices))}
	for i, s := range m.Sources {
		t := m.Statistics[i]
		p := 0.
		if s.Generated > 0 {
			p = float64(s.Refused) / float64(s.Generated)
		}
		r.Sources[i] = SourceResult{ID: i + 1, Name: s.Config.Name, Generated: s.Generated, Completed: s.Completed, Refused: s.Refused, Waiting: s.Waiting, Processing: s.Processing, RefusalProbability: p, MeanResidence: t.Residence.Mean, MeanWait: t.Wait.Mean, MeanService: t.Service.Mean, VarianceWait: t.Wait.Variance(), VarianceService: t.Service.Variance(), VarianceDefined: s.Completed >= 2}
		r.Completed += s.Completed
		r.Refused += s.Refused
	}
	if r.Generated > 0 {
		r.RefusalProbability = float64(r.Refused) / float64(r.Generated)
	}
	for i, d := range m.Devices {
		busy := d.BusyTime
		if d.Current != nil {
			busy += m.Now - d.Current.StartedAt
		}
		u := 0.
		if m.Now > 0 {
			u = busy / m.Now
		}
		r.Devices[i] = DeviceResult{ID: i + 1, Name: d.Config.Name, Completed: d.Completed, BusyTime: busy, Utilization: u}
	}
	return r
}
