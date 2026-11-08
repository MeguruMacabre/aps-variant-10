package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"aps-variant-10/internal/model"
)

func TestCLIInteractiveAndFixedAgree(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "step")
	b := filepath.Join(dir, "auto")
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"-n", "200", "-out", a}, strings.NewReader("n 2\ns\nt\nr\na\n"), &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if err := Run(context.Background(), []string{"-mode", "auto", "-fixed", "-n", "200", "-out", b}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	var x, y model.Experiment
	read := func(path string, v any) {
		d, err := os.ReadFile(filepath.Join(path, "summary.json"))
		if err != nil {
			t.Fatal(err)
		}
		if err = json.Unmarshal(d, v); err != nil {
			t.Fatal(err)
		}
	}
	read(a, &x)
	read(b, &y)
	if !reflect.DeepEqual(x, y) {
		t.Fatal("interactive/fixed results differ")
	}
	for _, name := range []string{"sources.csv", "devices.csv", "summary.json"} {
		if _, err := os.Stat(filepath.Join(a, name)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCLIValidationAndPartialExit(t *testing.T) {
	for _, args := range [][]string{{"-rate", "0"}, {"-n", "0"}, {"-min", "10", "-max", "1"}, {"-sources", "0"}, {"-mode", "bad"}, {"-demo", "-n", "8"}} {
		var b bytes.Buffer
		if err := Run(context.Background(), args, strings.NewReader("q\n"), &b, &b); err == nil {
			t.Fatal("accepted invalid flags", args)
		}
	}
	var b bytes.Buffer
	if err := Run(context.Background(), []string{"-demo"}, strings.NewReader("q\n"), &b, &b); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "ПРОМЕЖУТОЧНЫЙ РЕЗУЛЬТАТ") {
		t.Fatal("partial run presented as final")
	}
}
