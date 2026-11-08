package model

import (
	"context"
	"fmt"
	"math"
)

type Iteration struct {
	Number             int      `json:"iteration"`
	Requests           int      `json:"requests"`
	Seed               int64    `json:"seed"`
	Probability        float64  `json:"refusal_probability"`
	RequiredRequests   int      `json:"required_requests"`
	UnestimableSources []int    `json:"unestimable_sources"`
	RelativeChange     *float64 `json:"max_relative_change"`
	Met                bool     `json:"criteria_met"`
}
type PrecisionResult struct {
	Met           bool        `json:"criteria_met"`
	Reason        string      `json:"reason"`
	Confidence    float64     `json:"confidence"`
	RelativeError float64     `json:"relative_error"`
	Scope         string      `json:"scope"`
	Iterations    []Iteration `json:"iterations"`
}
type Experiment struct {
	Config    Config           `json:"config"`
	Summary   Report           `json:"summary"`
	Precision *PrecisionResult `json:"precision,omitempty"`
}

func RunFixed(ctx context.Context, c Config) (Experiment, error) {
	m, err := New(c)
	if err != nil {
		return Experiment{}, err
	}
	if err = m.Run(ctx); err != nil {
		return Experiment{}, err
	}
	return Experiment{Config: c, Summary: m.Report()}, nil
}

func RequiredCount(probability, relative, confidence float64) float64 {
	if probability <= 0 || probability >= 1 {
		return math.Inf(1)
	}
	z := math.Sqrt2 * math.Erfinv(confidence)
	return math.Ceil(z * z * (1 - probability) / (probability * relative * relative))
}

func RunAutomatic(ctx context.Context, c Config, progress func(Iteration)) (Experiment, error) {
	if err := c.Validate(); err != nil {
		return Experiment{}, err
	}
	if c.Requests > c.Precision.MaxRequests {
		return Experiment{}, fmt.Errorf("начальное число заявок превышает max_requests")
	}
	p := &PrecisionResult{Confidence: c.Precision.Confidence, RelativeError: c.Precision.Relative, Scope: "Приближённый подбор по формуле методички для вероятностей отказа общего потока и каждого источника"}
	n := c.Requests
	var prev *Report
	var result Experiment
	for i := 1; i <= c.Precision.MaxIterations; i++ {
		run := c
		run.Requests = n
		run.Seed = c.Seed + int64(i-1)*1000003
		x, err := RunFixed(ctx, run)
		if err != nil {
			return Experiment{}, err
		}
		r := x.Summary
		required := RequiredCount(r.RefusalProbability, c.Precision.Relative, c.Precision.Confidence)
		it := Iteration{Number: i, Requests: n, Seed: run.Seed, Probability: r.RefusalProbability}
		stable := prev != nil
		change := 0.
		compare := func(a, b float64) {
			if b <= 0 {
				stable = false
				return
			}
			v := math.Abs(a-b) / b
			if v > change {
				change = v
			}
		}
		if prev != nil {
			compare(r.RefusalProbability, prev.RefusalProbability)
		}
		for j, s := range r.Sources {
			if s.Refused == 0 || s.Generated == 0 || s.Refused == s.Generated {
				it.UnestimableSources = append(it.UnestimableSources, s.ID)
				required = math.Inf(1)
				stable = false
				continue
			}
			need := RequiredCount(s.RefusalProbability, c.Precision.Relative, c.Precision.Confidence) * float64(n) / float64(s.Generated)
			if need > required {
				required = need
			}
			if prev != nil {
				compare(s.RefusalProbability, prev.Sources[j].RefusalProbability)
			}
		}
		if finite(required) && required <= float64(c.Precision.MaxRequests) {
			it.RequiredRequests = int(math.Ceil(required))
		} else {
			it.RequiredRequests = c.Precision.MaxRequests + 1
		}
		if prev != nil && stable {
			v := change
			it.RelativeChange = &v
		}
		it.Met = stable && change <= c.Precision.Relative && float64(n) >= required
		p.Iterations = append(p.Iterations, it)
		if progress != nil {
			progress(it)
		}
		result = Experiment{Config: run, Summary: r, Precision: p}
		if it.Met {
			p.Met = true
			p.Reason = "Выполнены расчёт числа заявок и сравнение соседних прогонов для вероятностей отказа"
			return result, nil
		}
		if n >= c.Precision.MaxRequests {
			p.Reason = "Достигнут предел числа заявок; заданная точность не подтверждена"
			return result, nil
		}
		prev = &r
		next := 2 * n
		if finite(required) && required > float64(next) {
			if required > float64(c.Precision.MaxRequests) {
				next = c.Precision.MaxRequests
			} else {
				next = int(math.Ceil(required))
			}
		}
		if next > c.Precision.MaxRequests {
			next = c.Precision.MaxRequests
		}
		n = next
	}
	p.Reason = "Достигнут предел числа итераций; заданная точность не подтверждена"
	return result, nil
}
