package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/chef-guo/agents-hive/internal/agentquality"
	memoryeval "github.com/chef-guo/agents-hive/internal/memory/eval"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	dir := filepath.Join("internal", "agentquality", "testdata")
	var gateSummaryPath string
	var evalStaticSummaryPath string
	gateEval := false
	memoryEvalDir := filepath.Join("internal", "memory", "eval", "testdata")
	runMemoryEval := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--gate-summary":
			if i+1 >= len(args) {
				return fmt.Errorf("--gate-summary requires file path")
			}
			gateSummaryPath = args[i+1]
			i++
		case "--eval-static-summary":
			if i+1 >= len(args) {
				return fmt.Errorf("--eval-static-summary requires file path")
			}
			evalStaticSummaryPath = args[i+1]
			i++
		case "--eval-static":
			evalStaticSummaryPath = "-"
		case "--gate":
			gateEval = true
		case "--memory-eval":
			runMemoryEval = true
			if i+1 < len(args) && args[i+1] != "" && args[i+1][0] != '-' {
				memoryEvalDir = args[i+1]
				i++
			}
		default:
			dir = args[i]
		}
	}

	if runMemoryEval {
		if gateSummaryPath != "" || evalStaticSummaryPath != "" || gateEval {
			return fmt.Errorf("--memory-eval cannot be combined with agentquality gate options")
		}
		return runMemoryEvalCases(memoryEvalDir)
	}
	if gateSummaryPath != "" && evalStaticSummaryPath != "" {
		return fmt.Errorf("--gate-summary cannot be combined with --eval-static-summary")
	}

	cases, err := agentquality.LoadCases(dir)
	if err != nil {
		return err
	}
	for _, lc := range cases {
		if err := agentquality.ValidateCase(lc.Case); err != nil {
			return err
		}
	}
	fmt.Printf("agentquality schema ok: cases=%d\n", len(cases))

	if gateSummaryPath == "" {
		if evalStaticSummaryPath != "" {
			input := agentquality.StaticEvalSummary(cases)
			if err := writeEvalSummary(evalStaticSummaryPath, input); err != nil {
				return err
			}
			if gateEval {
				return printAndEvaluateGate(input, cases)
			}
		}
		return nil
	}
	input, err := readGateInput(gateSummaryPath)
	if err != nil {
		return err
	}
	return printAndEvaluateGate(input, cases)
}

func printAndEvaluateGate(input agentquality.GateInput, cases []agentquality.LoadedCase) error {
	input.Cases = cases
	metrics := agentquality.ComputeGateMetrics(input)
	b, err := json.MarshalIndent(metrics, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(b))
	if err := agentquality.EvaluateGate(metrics, agentquality.DefaultGateThresholds()); err != nil {
		fmt.Println("agentquality gate failed")
		return err
	}
	fmt.Println("agentquality gate pass")
	return nil
}

func writeEvalSummary(path string, input agentquality.GateInput) error {
	b, err := json.MarshalIndent(input, "", "  ")
	if err != nil {
		return err
	}
	if path == "-" {
		fmt.Println(string(b))
		return nil
	}
	return os.WriteFile(path, append(b, '\n'), 0o600)
}

func runMemoryEvalCases(dir string) error {
	summary, err := memoryeval.RunCases(context.Background(), dir)
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(b))
	if len(summary.RequiredFailed) > 0 {
		return fmt.Errorf("memory eval required cases failed: %v", summary.RequiredFailed)
	}
	fmt.Printf("memory eval pass: required=%d/%d total=%d/%d\n",
		summary.RequiredPassed,
		summary.RequiredTotal,
		summary.Passed,
		summary.Total,
	)
	return nil
}

func readGateInput(path string) (agentquality.GateInput, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return agentquality.GateInput{}, err
	}
	var input agentquality.GateInput
	if err := json.Unmarshal(b, &input); err != nil {
		return agentquality.GateInput{}, err
	}
	return input, nil
}
