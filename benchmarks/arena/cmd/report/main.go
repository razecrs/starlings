package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type row struct {
	Library      string  `json:"library"`
	Workload     string  `json:"workload"`
	Coverage     string  `json:"coverage"`
	NSPerOp      float64 `json:"ns_per_op"`
	PeakRSSBytes int64   `json:"peak_rss_bytes"`
}

type summary struct {
	Library, Workload, Coverage string
	Samples                     int
	Median, Low, High, MAD      float64
	RSS                         float64
	Relative                    float64
}

type adapterInfo struct {
	Library     string            `json:"library"`
	Supported   []string          `json:"supported"`
	Unsupported map[string]string `json:"unsupported"`
}

func main() {
	results := flag.String("results", "/opt/arena/results/final", "directory containing arena JSONL files")
	output := flag.String("out", "", "optional Markdown output path")
	flag.Parse()
	groups, err := load(*results)
	if err != nil {
		fatal(err)
	}
	summaries := make([]summary, 0, len(groups))
	for key, rows := range groups {
		parts := strings.Split(key, "\x00")
		values, rss := make([]float64, len(rows)), make([]float64, len(rows))
		for i, row := range rows {
			values[i], rss[i] = row.NSPerOp, float64(row.PeakRSSBytes)
		}
		median := percentile(values, .5)
		deviations := make([]float64, len(values))
		for i, value := range values {
			deviations[i] = math.Abs(value - median)
		}
		low, high := bootstrap(parts[0]+parts[1]+parts[2], values)
		summaries = append(summaries, summary{
			Library: parts[0], Workload: parts[1], Coverage: parts[2], Samples: len(rows),
			Median: median, Low: low, High: high, MAD: percentile(deviations, .5), RSS: percentile(rss, .5),
		})
	}
	fastest := make(map[string]float64)
	for _, value := range summaries {
		key := value.Workload + "\x00" + value.Coverage
		if current := fastest[key]; current == 0 || value.Median < current {
			fastest[key] = value.Median
		}
	}
	for i := range summaries {
		summaries[i].Relative = summaries[i].Median / fastest[summaries[i].Workload+"\x00"+summaries[i].Coverage]
	}
	sort.Slice(summaries, func(i, j int) bool {
		if summaries[i].Workload != summaries[j].Workload {
			return summaries[i].Workload < summaries[j].Workload
		}
		if summaries[i].Coverage != summaries[j].Coverage {
			return summaries[i].Coverage < summaries[j].Coverage
		}
		return summaries[i].Median < summaries[j].Median
	})
	var report bytes.Buffer
	report.WriteString("# Arena results\n\n")
	report.WriteString("Medians, median absolute deviation, and deterministic 95% bootstrap confidence intervals. Relative speed is calculated only inside an identical workload and coverage class.\n\n")
	report.WriteString("The recorded run used WSL2 on an i7-9750H. This repository retains the aggregate report, source locks, fixtures, and adapters, but not the original JSONL rows; rerun the arena before treating these numbers as independent verification.\n\n")
	report.WriteString("| Workload | Coverage | Library | Samples | Median | 95% CI | MAD | Peak RSS | Relative |\n")
	report.WriteString("|---|---|---|---:|---:|---:|---:|---:|---:|\n")
	for _, value := range summaries {
		fmt.Fprintf(&report, "| %s | %s | %s | %d | %s | %s–%s | %s | %s | %.2fx |\n",
			value.Workload, value.Coverage, value.Library, value.Samples,
			duration(value.Median), duration(value.Low), duration(value.High), duration(value.MAD), memory(value.RSS), value.Relative)
	}
	infos, err := loadInfo(*results)
	if err != nil {
		fatal(err)
	}
	if len(infos) > 0 {
		workloads := []string{"message_handled", "message_unhandled", "guild_create_state", "member_lookup", "permission_resolve"}
		sort.Slice(infos, func(i, j int) bool {
			left, right := benchmarkCoverage(infos[i].Supported, workloads), benchmarkCoverage(infos[j].Supported, workloads)
			if left != right {
				return left > right
			}
			return infos[i].Library < infos[j].Library
		})
		report.WriteString("\n## Public benchmark coverage\n\n")
		report.WriteString("A dash means the pinned library does not expose a supported public offline path for that workload; it was not assigned a fake score.\n\n")
		report.WriteString("| Library | Handled dispatch | Unhandled dispatch | Guild state | Member lookup | Permissions | Total |\n")
		report.WriteString("|---|:---:|:---:|:---:|:---:|:---:|---:|\n")
		for _, info := range infos {
			supported := make(map[string]bool, len(info.Supported))
			for _, workload := range info.Supported {
				supported[workload] = true
			}
			cells, total := make([]string, len(workloads)), 0
			for i, workload := range workloads {
				cells[i] = "—"
				if supported[workload] {
					cells[i] = "yes"
					total++
				}
			}
			fmt.Fprintf(&report, "| %s | %s | %s | %s | %s | %s | %d/5 |\n",
				info.Library, cells[0], cells[1], cells[2], cells[3], cells[4], total)
		}
	}
	if *output == "" {
		_, _ = os.Stdout.Write(report.Bytes())
		return
	}
	if err := os.WriteFile(*output, report.Bytes(), 0o644); err != nil {
		fatal(err)
	}
}

func benchmarkCoverage(supported, workloads []string) int {
	allowed := make(map[string]bool, len(workloads))
	for _, workload := range workloads {
		allowed[workload] = true
	}
	total := 0
	for _, workload := range supported {
		if allowed[workload] {
			total++
		}
	}
	return total
}

func loadInfo(directory string) ([]adapterInfo, error) {
	files, err := filepath.Glob(filepath.Join(directory, "*.info.json"))
	if err != nil {
		return nil, err
	}
	infos := make([]adapterInfo, 0, len(files))
	for _, name := range files {
		encoded, err := os.ReadFile(name)
		if err != nil {
			return nil, err
		}
		var info adapterInfo
		if err := json.Unmarshal(encoded, &info); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		if info.Library == "" {
			return nil, fmt.Errorf("%s: missing library", name)
		}
		infos = append(infos, info)
	}
	return infos, nil
}

func load(directory string) (map[string][]row, error) {
	files, err := filepath.Glob(filepath.Join(directory, "*.jsonl"))
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no JSONL results in %s", directory)
	}
	groups := make(map[string][]row)
	for _, name := range files {
		file, err := os.Open(name)
		if err != nil {
			return nil, err
		}
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			var value row
			if err := json.Unmarshal(scanner.Bytes(), &value); err != nil {
				_ = file.Close()
				return nil, fmt.Errorf("%s: %w", name, err)
			}
			if value.Library == "" || value.Workload == "" || value.Coverage == "" || value.NSPerOp < 0 {
				_ = file.Close()
				return nil, fmt.Errorf("%s: incomplete result row", name)
			}
			key := value.Library + "\x00" + value.Workload + "\x00" + value.Coverage
			groups[key] = append(groups[key], value)
		}
		if err := scanner.Err(); err != nil {
			_ = file.Close()
			return nil, err
		}
		_ = file.Close()
	}
	return groups, nil
}

func percentile(values []float64, quantile float64) float64 {
	copy := append([]float64(nil), values...)
	sort.Float64s(copy)
	if len(copy) == 0 {
		return 0
	}
	position := quantile * float64(len(copy)-1)
	lower := int(math.Floor(position))
	upper := int(math.Ceil(position))
	if lower == upper {
		return copy[lower]
	}
	return copy[lower] + (copy[upper]-copy[lower])*(position-float64(lower))
}

func bootstrap(key string, values []float64) (float64, float64) {
	seedHash := sha256.Sum256([]byte(key))
	random := rand.New(rand.NewSource(int64(binary.LittleEndian.Uint64(seedHash[:8]))))
	medians := make([]float64, 2000)
	sample := make([]float64, len(values))
	for i := range medians {
		for j := range sample {
			sample[j] = values[random.Intn(len(values))]
		}
		medians[i] = percentile(sample, .5)
	}
	return percentile(medians, .025), percentile(medians, .975)
}

func duration(ns float64) string {
	switch {
	case ns >= 1e9:
		return fmt.Sprintf("%.3f s", ns/1e9)
	case ns >= 1e6:
		return fmt.Sprintf("%.3f ms", ns/1e6)
	case ns >= 1e3:
		return fmt.Sprintf("%.3f µs", ns/1e3)
	default:
		return fmt.Sprintf("%.1f ns", ns)
	}
}

func memory(bytes float64) string {
	if bytes >= 1<<20 {
		return fmt.Sprintf("%.1f MiB", bytes/(1<<20))
	}
	if bytes >= 1<<10 {
		return fmt.Sprintf("%.1f KiB", bytes/(1<<10))
	}
	return fmt.Sprintf("%.0f B", bytes)
}

func fatal(err error) {
	_, _ = fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
