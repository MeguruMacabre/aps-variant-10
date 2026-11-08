package cli

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"

	"aps-variant-10/internal/model"
)

type options struct {
	mode, out   string
	demo, fixed bool
	config      model.Config
}

func parse(args []string, w io.Writer) (options, error) {
	c := model.DefaultConfig()
	f := flag.NewFlagSet("smo", flag.ContinueOnError)
	f.SetOutput(w)
	mode := f.String("mode", "step", "режим: step или auto")
	path := f.String("config", "", "JSON-файл параметров")
	demo := f.Bool("demo", false, "учебный сценарий нашей временной диаграммы")
	fixed := f.Bool("fixed", false, "один автоматический прогон без подбора точности")
	out := f.String("out", "", "каталог для CSV и JSON результатов")
	sources := f.Int("sources", 3, "число источников")
	devices := f.Int("devices", 2, "число приборов")
	buffer := f.Int("buffer", 3, "число мест буфера")
	min := f.Float64("min", 1, "минимальный интервал всех источников")
	max := f.Float64("max", 10, "максимальный интервал всех источников")
	rate := f.Float64("rate", .25, "интенсивность обслуживания каждого прибора")
	n := f.Int("n", 1000, "число заявок; в подборе точности — начальный объём")
	seed := f.Int64("seed", 42, "начальное значение генераторов случайных чисел")
	maxN := f.Int("max-n", 2000000, "предел числа заявок для подбора точности")
	iterations := f.Int("iterations", 12, "предел числа итераций подбора")
	if err := f.Parse(args); err != nil {
		return options{}, err
	}
	if f.NArg() > 0 {
		return options{}, fmt.Errorf("лишние аргументы: %s", strings.Join(f.Args(), " "))
	}
	if *mode != "step" && *mode != "auto" {
		return options{}, fmt.Errorf("mode должен быть step или auto")
	}
	if *path != "" {
		var err error
		c, err = model.LoadConfig(*path)
		if err != nil {
			return options{}, err
		}
	}
	seen := map[string]bool{}
	f.Visit(func(x *flag.Flag) { seen[x.Name] = true })
	if seen["sources"] {
		if *sources < 1 || *sources > 1000 {
			return options{}, fmt.Errorf("источников должно быть от 1 до 1000")
		}
		c.Sources = make([]model.SourceConfig, *sources)
		for i := range c.Sources {
			c.Sources[i] = model.SourceConfig{Name: fmt.Sprintf("Камера %d", i+1), Min: 1, Max: 10}
		}
	}
	if seen["devices"] {
		if *devices < 1 || *devices > 1000 {
			return options{}, fmt.Errorf("приборов должно быть от 1 до 1000")
		}
		c.Devices = make([]model.DeviceConfig, *devices)
		for i := range c.Devices {
			c.Devices[i] = model.DeviceConfig{Name: fmt.Sprintf("Обработчик %d", i+1), Rate: .25}
		}
	}
	for i := range c.Sources {
		if seen["min"] {
			c.Sources[i].Min = *min
		}
		if seen["max"] {
			c.Sources[i].Max = *max
		}
	}
	for i := range c.Devices {
		if seen["rate"] {
			c.Devices[i].Rate = *rate
		}
	}
	if seen["buffer"] {
		c.BufferSize = *buffer
	}
	if seen["n"] {
		c.Requests = *n
	}
	if seen["seed"] {
		c.Seed = *seed
	}
	if seen["max-n"] {
		c.Precision.MaxRequests = *maxN
	}
	if seen["iterations"] {
		c.Precision.MaxIterations = *iterations
	}
	if *demo {
		for _, name := range []string{"config", "sources", "devices", "buffer", "min", "max", "rate", "n", "seed", "max-n", "iterations"} {
			if seen[name] {
				return options{}, fmt.Errorf("-demo имеет фиксированные параметры; не совмещайте его с -%s", name)
			}
		}
	}
	if err := c.Validate(); err != nil {
		return options{}, err
	}
	return options{*mode, *out, *demo, *fixed, c}, nil
}

func Run(ctx context.Context, args []string, in io.Reader, out, errOut io.Writer) error {
	o, err := parse(args, errOut)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
	}
	fmt.Fprintln(out, "Вариант 10 — обработка снимков с камер наблюдения")
	var result model.Experiment
	if o.demo || o.mode == "step" {
		makeModel := func() (*model.Simulation, error) {
			if o.demo {
				return model.NewDemo()
			}
			return model.New(o.config)
		}
		m, e := makeModel()
		if e != nil {
			return e
		}
		m.Detailed = true
		PrintConfig(out, m.Config)
		if o.demo {
			fmt.Fprintln(out, "ДЕМО: 8 заявок с заданными временами. Это проверочный сценарий, а не случайный эксперимент.")
		}
		if o.mode == "auto" {
			m.Detailed = false
			if err = m.Run(ctx); err != nil {
				return err
			}
		} else {
			PrintState(out, m)
			printHelp(out)
			scanner := bufio.NewScanner(in)
			for !m.Done() {
				fmt.Fprint(out, "Команда> ")
				if !scanner.Scan() {
					if scanner.Err() != nil {
						return scanner.Err()
					}
					fmt.Fprintln(out, "\nВвод завершён. Показан результат незаконченного прогона.")
					break
				}
				parts := strings.Fields(strings.ToLower(scanner.Text()))
				cmd := "n"
				if len(parts) > 0 {
					cmd = parts[0]
				}
				if cmd == "q" || cmd == "quit" {
					fmt.Fprintln(out, "Прогон остановлен пользователем.")
					break
				}
				switch cmd {
				case "h", "help", "?":
					printHelp(out)
				case "s", "state":
					PrintState(out, m)
				case "t", "stats":
					PrintReport(out, m.Report())
				case "r", "reset":
					m, err = makeModel()
					if err != nil {
						return err
					}
					m.Detailed = true
					PrintState(out, m)
				case "a", "auto":
					if err = m.Run(ctx); err != nil {
						return err
					}
					PrintState(out, m)
				case "n", "next":
					count := 1
					if len(parts) > 2 {
						fmt.Fprintln(out, "Формат: n или n 5")
						continue
					}
					if len(parts) == 2 {
						count, err = strconv.Atoi(parts[1])
						if err != nil || count < 1 || count > 1000000 {
							fmt.Fprintln(out, "Число шагов должно быть от 1 до 1000000")
							continue
						}
					}
					for i := 0; i < count && !m.Done(); i++ {
						select {
						case <-ctx.Done():
							return ctx.Err()
						default:
						}
						if _, err = m.Step(); err != nil {
							return err
						}
					}
					PrintState(out, m)
				default:
					fmt.Fprintln(out, "Неизвестная команда. Введите h.")
				}
			}
		}
		result = model.Experiment{Config: m.Config, Summary: m.Report()}
	} else {
		PrintConfig(out, o.config)
		if o.fixed {
			fmt.Fprintln(out, "Фиксированный прогон: подбор статистической точности отключён.")
			result, err = model.RunFixed(ctx, o.config)
		} else {
			fmt.Fprintf(out, "Подбор числа заявок: относительная точность %.1f%%, доверительная вероятность %.3g\n", 100*o.config.Precision.Relative, o.config.Precision.Confidence)
			result, err = model.RunAutomatic(ctx, o.config, func(i model.Iteration) {
				change := "—"
				if i.RelativeChange != nil {
					change = fmt.Sprintf("%.2f%%", 100**i.RelativeChange)
				}
				need := strconv.Itoa(i.RequiredRequests)
				if i.RequiredRequests > o.config.Precision.MaxRequests {
					need = "выше лимита / пока не оценивается"
				}
				fmt.Fprintf(out, "Итерация %d: N=%d; seed=%d; pотк=%.6f; нужно N=%s; изменение=%s\n", i.Number, i.Requests, i.Seed, i.Probability, need, change)
				if len(i.UnestimableSources) > 0 {
					fmt.Fprintf(out, "  Не оценить относительную точность для источников %v: нет отказов или недостаточно наблюдений.\n", i.UnestimableSources)
				}
			})
		}
		if err != nil {
			return err
		}
	}
	PrintReport(out, result.Summary)
	if result.Precision != nil {
		fmt.Fprintln(out, "\nПодбор точности:", result.Precision.Reason)
		fmt.Fprintln(out, "Область проверки: вероятности отказа общего потока и каждого источника.")
		fmt.Fprintln(out, "Использована приближённая формула методички; это не отдельное подтверждение точности средних времён и дисперсий.")
	}
	if o.out != "" {
		if err = Export(o.out, result); err != nil {
			return err
		}
		fmt.Fprintln(out, "Результаты сохранены в", o.out)
	}
	return nil
}

func printHelp(w io.Writer) {
	fmt.Fprintln(w, "Команды: Enter/n — событие; n 5 — пять событий; s — состояние; t — статистика; a — до конца; r — сброс; q — выход; h — помощь.")
}
