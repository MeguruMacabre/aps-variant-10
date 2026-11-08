package cli

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"aps-variant-10/internal/model"
)

func table(w io.Writer) *tabwriter.Writer { return tabwriter.NewWriter(w, 0, 4, 2, ' ', 0) }
func PrintConfig(w io.Writer, c model.Config) {
	fmt.Fprintf(w, "Источников: %d; приборов: %d; мест буфера: %d; заявок: %d; seed: %d\n", len(c.Sources), len(c.Devices), c.BufferSize, c.Requests, c.Seed)
	t := table(w)
	fmt.Fprintln(t, "Источник\tНазвание\tИнтервал поступления")
	for i, s := range c.Sources {
		fmt.Fprintf(t, "И%d\t%s\tUniform(%g, %g)\n", i+1, s.Name, s.Min, s.Max)
	}
	t.Flush()
	t = table(w)
	fmt.Fprintln(t, "Прибор\tИнтенсивность mu\tСреднее обслуживание 1/mu")
	for i, d := range c.Devices {
		fmt.Fprintf(t, "П%d\t%g\t%g\n", i+1, d.Rate, 1/d.Rate)
	}
	t.Flush()
}
func requestID(r *model.Request) string {
	if r == nil {
		return "—"
	}
	return r.ID
}
func PrintState(w io.Writer, m *model.Simulation) {
	fmt.Fprintf(w, "\n=== Шаг %d; модельное время %.6g; поступило %d/%d ===\n", m.Steps, m.Now, m.Generated, m.Config.Requests)
	if m.Last != nil {
		fmt.Fprintln(w, "Событие:", m.Last.Label())
		for _, s := range m.Changes {
			fmt.Fprintln(w, "  "+s)
		}
	}
	fmt.Fprintln(w, "Календарь событий:")
	t := table(w)
	fmt.Fprintln(t, "Событие\tВремя\tЗаявка")
	for _, e := range m.Events() {
		id := "—"
		if e.Kind == model.ServiceCompletion {
			id = requestID(m.Devices[e.Index].Current)
		}
		fmt.Fprintf(t, "%s\t%.6g\t%s\n", e.Label(), e.Time, id)
	}
	t.Flush()
	if m.Done() {
		fmt.Fprintln(w, "Календарь пуст. Все принятые заявки обработаны.")
	}
	fmt.Fprintln(w, "Буфер:")
	t = table(w)
	fmt.Fprintln(t, "Место\tЗаявка\tПоступила\tЖдёт")
	for i, r := range m.Buffer.Slots {
		if r == nil {
			fmt.Fprintf(t, "Б%d\t—\t—\t—\n", i+1)
		} else {
			fmt.Fprintf(t, "Б%d\t%s\t%.6g\t%.6g\n", i+1, r.ID, r.GeneratedAt, m.Now-r.GeneratedAt)
		}
	}
	t.Flush()
	if len(m.Buffer.Slots) == 0 {
		fmt.Fprintln(w, "Буфер отсутствует (0 мест).")
	}
	fmt.Fprintln(w, "Приборы:")
	t = table(w)
	fmt.Fprintln(t, "Прибор\tЗаявка\tНачало\tЗавершение")
	for i, d := range m.Devices {
		if d.Current == nil {
			fmt.Fprintf(t, "П%d\tсвободен\t—\t—\n", i+1)
		} else {
			fmt.Fprintf(t, "П%d\t%s\t%.6g\t%.6g\n", i+1, d.Current.ID, d.Current.StartedAt, d.FinishAt)
		}
	}
	t.Flush()
	if m.Selection.ActivePacket == nil {
		fmt.Fprintln(w, "Пакет: ещё не сформирован.")
	} else {
		ids := m.PacketIDs()
		text := "исчерпан"
		if len(ids) > 0 {
			text = strings.Join(ids, ", ")
		}
		fmt.Fprintf(w, "Пакет И%d: %s\n", m.Selection.ActivePacket.SourceID+1, text)
	}
	fmt.Fprintf(w, "Поиск прибора начинается с П%d; занятые пропускаются.\n", m.Selection.NextDevice+1)
	t = table(w)
	fmt.Fprintln(t, "Источник\tПоступило\tОбслужено\tОтказ\tВ буфере\tНа приборе")
	for i, s := range m.Sources {
		fmt.Fprintf(t, "И%d\t%d\t%d\t%d\t%d\t%d\n", i+1, s.Generated, s.Completed, s.Refused, s.Waiting, s.Processing)
	}
	t.Flush()
}
func PrintReport(w io.Writer, r model.Report) {
	state := "ИТОГ"
	if !r.Finished {
		state = "ПРОМЕЖУТОЧНЫЙ РЕЗУЛЬТАТ"
	}
	fmt.Fprintf(w, "\n=== %s; время %.6g; событий %d ===\n", state, r.Time, r.Steps)
	fmt.Fprintf(w, "Поступило: %d; обслужено: %d; отказано: %d; pотк = %.6f\n", r.Generated, r.Completed, r.Refused, r.RefusalProbability)
	fmt.Fprintln(w, "Таблица 1. Источники. Времена — по обслуженным заявкам; дисперсии — выборочные.")
	t := table(w)
	fmt.Fprintln(t, "Источник\tПоступило\tОбсл.\tОтказ\tpотк\tTсист\tTож\tTобсл\tDож\tDобсл")
	for _, s := range r.Sources {
		vw, vs := "—", "—"
		if s.VarianceDefined {
			vw = fmt.Sprintf("%.6g", s.VarianceWait)
			vs = fmt.Sprintf("%.6g", s.VarianceService)
		}
		res, wait, service := "—", "—", "—"
		if s.Completed > 0 {
			res = fmt.Sprintf("%.6g", s.MeanResidence)
			wait = fmt.Sprintf("%.6g", s.MeanWait)
			service = fmt.Sprintf("%.6g", s.MeanService)
		}
		prob := "—"
		if s.Generated > 0 {
			prob = fmt.Sprintf("%.6f", s.RefusalProbability)
		}
		fmt.Fprintf(t, "И%d\t%d\t%d\t%d\t%s\t%s\t%s\t%s\t%s\t%s\n", s.ID, s.Generated, s.Completed, s.Refused, prob, res, wait, service, vw, vs)
	}
	t.Flush()
	fmt.Fprintln(w, "Таблица 2. Приборы.")
	t = table(w)
	fmt.Fprintln(t, "Прибор\tОбслужено\tВремя занятости\tКоэффициент использования")
	for _, d := range r.Devices {
		fmt.Fprintf(t, "П%d\t%d\t%.6g\t%.6f\n", d.ID, d.Completed, d.BusyTime, d.Utilization)
	}
	t.Flush()
	fmt.Fprintln(w, "Единица времени условная. «—» означает недостаток наблюдений.")
}
