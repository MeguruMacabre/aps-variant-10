package cli

import (
	"encoding/csv"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"

	"aps-variant-10/internal/model"
)

func number(x float64) string { return strconv.FormatFloat(x, 'g', -1, 64) }
func integer(x int) string    { return strconv.Itoa(x) }
func writeCSV(path string, rows [][]string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	w := csv.NewWriter(f)
	err = w.WriteAll(rows)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}
func Export(dir string, x model.Experiment) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(x, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if err = os.WriteFile(filepath.Join(dir, "summary.json"), b, 0644); err != nil {
		return err
	}
	rows := [][]string{{"source_id", "name", "generated", "completed", "refused", "waiting", "processing", "refusal_probability", "mean_residence", "mean_wait", "mean_service", "variance_wait", "variance_service"}}
	for _, s := range x.Summary.Sources {
		p := ""
		if s.Generated > 0 {
			p = number(s.RefusalProbability)
		}
		a, b, c, d, e := "", "", "", "", ""
		if s.Completed > 0 {
			a = number(s.MeanResidence)
			b = number(s.MeanWait)
			c = number(s.MeanService)
		}
		if s.VarianceDefined {
			d = number(s.VarianceWait)
			e = number(s.VarianceService)
		}
		rows = append(rows, []string{integer(s.ID), s.Name, integer(s.Generated), integer(s.Completed), integer(s.Refused), integer(s.Waiting), integer(s.Processing), p, a, b, c, d, e})
	}
	if err = writeCSV(filepath.Join(dir, "sources.csv"), rows); err != nil {
		return err
	}
	rows = [][]string{{"device_id", "name", "completed", "busy_time", "utilization"}}
	for _, d := range x.Summary.Devices {
		rows = append(rows, []string{integer(d.ID), d.Name, integer(d.Completed), number(d.BusyTime), number(d.Utilization)})
	}
	if err = writeCSV(filepath.Join(dir, "devices.csv"), rows); err != nil {
		return err
	}
	path := filepath.Join(dir, "iterations.csv")
	if x.Precision == nil {
		if err = os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	rows = [][]string{{"iteration", "requests", "seed", "refusal_probability", "required_requests", "max_relative_change", "criteria_met"}}
	for _, i := range x.Precision.Iterations {
		change := ""
		if i.RelativeChange != nil {
			change = number(*i.RelativeChange)
		}
		rows = append(rows, []string{integer(i.Number), integer(i.Requests), strconv.FormatInt(i.Seed, 10), number(i.Probability), integer(i.RequiredRequests), change, strconv.FormatBool(i.Met)})
	}
	return writeCSV(path, rows)
}
