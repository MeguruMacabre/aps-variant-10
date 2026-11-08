package model

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
)

type SourceConfig struct {
	Name string  `json:"name"`
	Min  float64 `json:"min_interval"`
	Max  float64 `json:"max_interval"`
}

type DeviceConfig struct {
	Name string  `json:"name"`
	Rate float64 `json:"rate"`
}

type PrecisionConfig struct {
	Relative      float64 `json:"relative_error"`
	Confidence    float64 `json:"confidence"`
	MaxRequests   int     `json:"max_requests"`
	MaxIterations int     `json:"max_iterations"`
}

type Config struct {
	Sources    []SourceConfig  `json:"sources"`
	Devices    []DeviceConfig  `json:"devices"`
	BufferSize int             `json:"buffer_size"`
	Requests   int             `json:"requests"`
	Seed       int64           `json:"seed"`
	Precision  PrecisionConfig `json:"precision"`
}

func DefaultConfig() Config {
	return Config{
		Sources:    []SourceConfig{{"Въезд", 1, 10}, {"Погрузочная площадка", 1, 10}, {"Стоянка", 1, 10}},
		Devices:    []DeviceConfig{{"Обработчик 1", .25}, {"Обработчик 2", .25}},
		BufferSize: 3, Requests: 1000, Seed: 42,
		Precision: PrecisionConfig{.1, .9, 2000000, 12},
	}
}

func LoadConfig(path string) (Config, error) {
	c := DefaultConfig()
	f, err := os.Open(path)
	if err != nil {
		return c, err
	}
	defer f.Close()
	d := json.NewDecoder(f)
	d.DisallowUnknownFields()
	if err = d.Decode(&c); err != nil {
		return c, fmt.Errorf("конфигурация: %w", err)
	}
	var extra any
	if err = d.Decode(&extra); err != io.EOF {
		return c, fmt.Errorf("после конфигурации обнаружены лишние данные")
	}
	return c, c.Validate()
}

func finite(x float64) bool { return !math.IsNaN(x) && !math.IsInf(x, 0) }

func (c Config) Validate() error {
	if len(c.Sources) < 1 || len(c.Sources) > 1000 {
		return fmt.Errorf("источников должно быть от 1 до 1000")
	}
	if len(c.Devices) < 1 || len(c.Devices) > 1000 {
		return fmt.Errorf("приборов должно быть от 1 до 1000")
	}
	if c.BufferSize < 0 || c.BufferSize > 100000 {
		return fmt.Errorf("размер буфера должен быть от 0 до 100000")
	}
	if c.Requests < 1 || c.Requests > 100000000 {
		return fmt.Errorf("число заявок должно быть от 1 до 100000000")
	}
	for i, s := range c.Sources {
		if !finite(s.Min) || !finite(s.Max) || s.Min <= 0 || s.Max <= s.Min {
			return fmt.Errorf("И%d: нужны конечные интервалы 0 < min < max", i+1)
		}
	}
	for i, d := range c.Devices {
		if !finite(d.Rate) || d.Rate <= 0 {
			return fmt.Errorf("П%d: интенсивность должна быть конечной и положительной", i+1)
		}
	}
	p := c.Precision
	if !finite(p.Relative) || p.Relative <= 0 || p.Relative >= 1 {
		return fmt.Errorf("relative_error должен быть между 0 и 1")
	}
	if !finite(p.Confidence) || p.Confidence <= 0 || p.Confidence >= 1 {
		return fmt.Errorf("confidence должен быть между 0 и 1")
	}
	if p.MaxRequests < 1 || p.MaxRequests > 100000000 || p.MaxIterations < 2 || p.MaxIterations > 100 {
		return fmt.Errorf("некорректные ограничения автоматического подбора")
	}
	return nil
}
