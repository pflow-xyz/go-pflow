package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/pflow-xyz/go-pflow/parser"
	"github.com/pflow-xyz/go-pflow/verify"
)

// verifyCmd checks declarative properties against a model and reports
// proved / refuted / unknown with evidence.
func verifyCmd(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ExitOnError)

	var props stringList
	fs.Var(&props, "p", "property to check (repeatable); see forms below")
	fs.Var(&props, "property", "alias for -p")

	maxStates := fs.Int("max-states", verify.DefaultMaxStates, "state exploration limit")
	outputJSON := fs.Bool("json", false, "output results as JSON")
	outputFile := fs.String("output", "", "write results to file")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage: pflow verify <model.json> -p <property> [-p <property> ...]

Check declarative properties against a Petri net. Each property returns
proved, refuted, or unknown — refutations carry a replayable firing sequence.

Property forms:
  deadlock-free              no reachable marking is a deadlock
  bounded                    no place accumulates tokens without limit
  live                       every transition can fire from some marking
  terminating                every execution eventually stops
  conserves                  total token count never changes
  reachable:<marking>        some reachable marking matches, e.g. reachable:done=1
  unreachable:<marking>      no reachable marking matches (safety)
  mutex:<p1,p2,...>[<=n]     at most n of these places hold a token at once
  <linear expression>        holds at every reachable marking, e.g. "a + 2*b == 10"

A verdict's method says how far it generalizes:
  structural   proved by linear algebra — holds for ANY initial marking
  exhaustive   proved by enumerating this marking's full state space
  witness      decided by a constructive witness (e.g. an unbounded pump)
  partial      exploration was truncated; only refutations are sound

Examples:
  pflow verify model.json -p deadlock-free -p bounded
  pflow verify model.json -p "mutex:busy1,busy2"
  pflow verify model.json -p "minted == circulating + burned"
  pflow verify model.json -p unreachable:busy1=1,busy2=1 --json
`)
	}

	if err := fs.Parse(reorderArgs(fs, args)); err != nil {
		return err
	}

	if fs.NArg() < 1 {
		fs.Usage()
		return fmt.Errorf("model file required")
	}
	if len(props) == 0 {
		fs.Usage()
		return fmt.Errorf("at least one -p property required")
	}

	jsonData, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		return fmt.Errorf("read model: %w", err)
	}

	net, err := parser.FromJSON(jsonData)
	if err != nil {
		return fmt.Errorf("parse model: %w", err)
	}

	parsed := make([]verify.Property, 0, len(props))
	for _, spec := range props {
		p, err := parseProperty(spec)
		if err != nil {
			return fmt.Errorf("property %q: %w", spec, err)
		}
		parsed = append(parsed, p)
	}

	report := verify.New(net).WithMaxStates(*maxStates).Check(parsed...)

	if *outputJSON || *outputFile != "" {
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal JSON: %w", err)
		}
		if *outputFile != "" {
			if err := os.WriteFile(*outputFile, data, 0644); err != nil {
				return fmt.Errorf("write file: %w", err)
			}
			fmt.Fprintf(os.Stderr, "Verification report written to %s\n", *outputFile)
		} else {
			fmt.Println(string(data))
		}
	} else {
		printVerifyReport(report)
	}

	// Non-zero exit unless everything was proved, so this is usable as a gate.
	if !report.OK {
		os.Exit(1)
	}
	return nil
}

// parseProperty converts a CLI property spec into a verify.Property.
func parseProperty(spec string) (verify.Property, error) {
	spec = strings.TrimSpace(spec)

	switch strings.ToLower(spec) {
	case "deadlock-free", "deadlockfree":
		return verify.Property{Kind: verify.KindDeadlockFree, Name: spec}, nil
	case "bounded":
		return verify.Property{Kind: verify.KindBounded, Name: spec}, nil
	case "live", "liveness":
		return verify.Property{Kind: verify.KindLive, Name: spec}, nil
	case "terminating", "terminates":
		return verify.Property{Kind: verify.KindTerminating, Name: spec}, nil
	case "conserves", "conservation":
		return verify.Property{Kind: verify.KindConserves, Name: spec}, nil
	}

	switch {
	case strings.HasPrefix(spec, "reachable:"):
		target, err := parseMarkingSpec(strings.TrimPrefix(spec, "reachable:"))
		if err != nil {
			return verify.Property{}, err
		}
		return verify.Property{Kind: verify.KindReachable, Name: spec, Target: target}, nil

	case strings.HasPrefix(spec, "unreachable:"):
		target, err := parseMarkingSpec(strings.TrimPrefix(spec, "unreachable:"))
		if err != nil {
			return verify.Property{}, err
		}
		return verify.Property{Kind: verify.KindUnreachable, Name: spec, Target: target}, nil

	case strings.HasPrefix(spec, "mutex:"):
		body := strings.TrimPrefix(spec, "mutex:")
		bound := 1
		if idx := strings.Index(body, "<="); idx >= 0 {
			n, err := strconv.Atoi(strings.TrimSpace(body[idx+2:]))
			if err != nil {
				return verify.Property{}, fmt.Errorf("bad bound after '<=': %w", err)
			}
			bound = n
			body = body[:idx]
		}
		var places []string
		for _, p := range strings.Split(body, ",") {
			if p = strings.TrimSpace(p); p != "" {
				places = append(places, p)
			}
		}
		if len(places) == 0 {
			return verify.Property{}, fmt.Errorf("mutex requires at least one place")
		}
		return verify.Property{Kind: verify.KindMutualExclusion, Name: spec, Places: places, Bound: bound}, nil
	}

	// Anything else is treated as a linear expression; ParseExpr validates it
	// here so a typo surfaces before any analysis runs.
	if _, err := verify.ParseExpr(spec); err != nil {
		return verify.Property{}, err
	}
	return verify.Property{Kind: verify.KindInvariant, Name: spec, Expr: spec}, nil
}

// parseMarkingSpec parses "a=1,b=0" into a partial marking.
func parseMarkingSpec(s string) (map[string]int, error) {
	target := make(map[string]int)
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			return nil, fmt.Errorf("expected place=tokens, got %q", part)
		}
		n, err := strconv.Atoi(strings.TrimSpace(kv[1]))
		if err != nil {
			return nil, fmt.Errorf("bad token count in %q: %w", part, err)
		}
		target[strings.TrimSpace(kv[0])] = n
	}
	if len(target) == 0 {
		return nil, fmt.Errorf("empty marking")
	}
	return target, nil
}

func printVerifyReport(report *verify.Report) {
	fmt.Println("=== Petri Net Verification ===")
	fmt.Println()

	for _, v := range report.Verdicts {
		var mark string
		switch v.Status {
		case verify.Proved:
			mark = "✓ PROVED  "
		case verify.Refuted:
			mark = "✗ REFUTED "
		default:
			mark = "? UNKNOWN "
		}

		name := v.Property.Name
		if name == "" {
			name = string(v.Property.Kind)
		}

		fmt.Printf("%s %s\n", mark, name)
		if v.Method != "" {
			fmt.Printf("            method: %s\n", v.Method)
		}
		if v.Detail != "" {
			fmt.Printf("            %s\n", v.Detail)
		}
		if v.Evidence != "" {
			fmt.Printf("            evidence: %s\n", v.Evidence)
		}
		if ce := v.Counterexample; ce != nil {
			if len(ce.Trace) > 0 {
				fmt.Printf("            trace: %s\n", strings.Join(ce.Trace, " → "))
			}
			fmt.Printf("            marking: %v\n", ce.Marking)
			if ce.Explanation != "" {
				fmt.Printf("            why: %s\n", ce.Explanation)
			}
		}
		fmt.Println()
	}

	fmt.Println(report.Summary())
}

// stringList collects a repeatable string flag.
type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ", ") }

func (s *stringList) Set(v string) error {
	*s = append(*s, v)
	return nil
}
