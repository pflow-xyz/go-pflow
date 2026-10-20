// Custody withdrawal as a saga: reserve funds, sign, broadcast — and, if any
// step fails, undo the ones that ran in reverse.
//
//	go run ./examples/custody_saga
package main

import (
	"fmt"
	"os"

	"github.com/pflow-xyz/go-pflow/metamodel"
	"github.com/pflow-xyz/go-pflow/metamodel/metapetri"
	"github.com/pflow-xyz/go-pflow/reachability"
	"github.com/pflow-xyz/go-pflow/saga"
	"github.com/pflow-xyz/go-pflow/verify"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "custody_saga:", err)
		os.Exit(1)
	}
}

func run() error {
	fmt.Println("== standalone: the saga owns every account ==")
	b := metamodel.NewBundle("custody")
	b.AddSubnet(*Withdrawal(100, 40).ToMetaSubnet())
	if err := report(b); err != nil {
		return err
	}

	for _, confirmWithCommit := range []bool{false, true} {
		fmt.Printf("\n== composed with a ledger and a chain (confirm fused to commit: %v) ==\n", confirmWithCommit)
		if err := report(Compose(100, 40, confirmWithCommit)); err != nil {
			return err
		}
	}
	return nil
}

func report(b *metamodel.Bundle) error {
	res, err := metapetri.ConvertBundle(b, metapetri.Options{})
	if err != nil {
		return err
	}
	space := reachability.NewAnalyzer(res.Net).WithInitialMarking(res.Marking).WithMaxStates(100000).Analyze()
	fmt.Printf("states: %d  edges: %d  complete: %v\n", space.StateCount, space.EdgeCount, space.IsComplete)

	w := res.FlatMap.Place["withdrawal"]
	stranded := saga.Stranded(space.Graph, w["completed"], w["rolled_back"])
	fmt.Printf("stranded states (cannot reach rolled_back or completed): %d\n", len(stranded))
	if len(stranded) > 0 {
		fmt.Printf("  e.g. %s\n", stranded[0].Marking)
	}

	rep, err := metapetri.Verify(res,
		verify.Property{Kind: verify.KindLive, Name: "no dead transitions"},
		verify.Property{Kind: verify.KindMutualExclusion, Name: "completed and rolled_back are exclusive",
			Places: []string{w["completed"], w["rolled_back"]}, Bound: 1},
	)
	if err != nil {
		return err
	}
	for _, vd := range rep.Verdicts {
		fmt.Printf("%-40s %s (%s)\n", vd.Property.Name, vd.Status, vd.Method)
	}
	return nil
}
