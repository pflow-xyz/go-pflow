// Command zk-field-parity emits a deterministic digest of BN254 field
// arithmetic and gnark circuit artifacts.
//
// Why this exists (F4 — the asm↔purego breach):
//
// Bazel builds petri-pilot hermetically with `-tags purego`, selecting
// gnark-crypto's pure-Go field arithmetic, because the amd64/arm64 assembly
// uses relative cross-package `#include` directives that don't resolve in
// Bazel's sandbox. `make build` / `go build` — the binary actually shipped to
// pilot.pflow.xyz and the one that produces the on-chain Groth16 verifiers —
// use the asm fast path instead. The .bazelrc comment asserts the two are
// "bit-identical, just slower"; nothing tested that claim.
//
// This binary makes the claim falsifiable. Build it twice — once with `purego`
// and once without — run both, and diff stdout. Identical output is positive
// evidence the two field backends agree on the surface that matters: raw
// Fp/Fr arithmetic (Mul/Square/Inverse/Exp/Sqrt live in element_amd64.go vs
// element_purego.go — the literal boundary), native MiMC, the compiled R1CS of
// a MiMC circuit, and a solved witness vector. A single differing byte means a
// hidden backend divergence exists. See scripts/zk-parity-check.sh.
//
// Only deterministic surfaces are hashed. groth16.Setup/Prove sample
// crypto/rand, so proving keys and proofs are NOT reproducible across runs and
// are deliberately excluded; the R1CS, the solved witness, and the raw field
// vectors are fully determined by their inputs and are what a backend
// divergence would corrupt.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"math/big"
	"os"

	"github.com/consensys/gnark-crypto/ecc"
	bn254fp "github.com/consensys/gnark-crypto/ecc/bn254/fp"
	bn254fr "github.com/consensys/gnark-crypto/ecc/bn254/fr"
	bn254mimc "github.com/consensys/gnark-crypto/ecc/bn254/fr/mimc"
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/frontend/cs/r1cs"
	"github.com/consensys/gnark/logger"
	"github.com/consensys/gnark/std/hash/mimc"
)

// iterations for the field-op battery; large enough to chain every op many
// times, small enough to stay instant.
const iterations = 2000

func main() {
	// gnark's compiler logs progress (and a non-deterministic solve timing) to
	// stdout; silence it so only the parity digests are emitted.
	logger.Disable()

	sections := []struct {
		name string
		fn   func(hash.Hash)
	}{
		{"field-fr", frBattery},
		{"field-fp", fpBattery},
		{"mimc-native", mimcNative},
		{"r1cs", r1csDigest},
		{"witness", witnessDigest},
	}

	combined := sha256.New()
	for _, s := range sections {
		h := sha256.New()
		s.fn(h)
		sum := h.Sum(nil)
		fmt.Printf("%-12s %s\n", s.name, hex.EncodeToString(sum))
		combined.Write([]byte(s.name))
		combined.Write(sum)
	}
	fmt.Printf("%-12s %s\n", "COMBINED", hex.EncodeToString(combined.Sum(nil)))
}

// frBattery chains the asm-accelerated Fr operations and feeds every
// intermediate result into h. Inverse/Exp/Sqrt are exercised explicitly.
func frBattery(h hash.Hash) {
	var a, b, c bn254fr.Element
	a.SetUint64(2)
	b.SetUint64(3)
	for i := 0; i < iterations; i++ {
		c.Mul(&a, &b)
		c.Square(&c)
		c.Add(&c, &a)
		c.Sub(&c, &b)
		c.Double(&c)
		c.Neg(&c)
		if i%50 == 0 {
			var inv bn254fr.Element
			inv.Inverse(&b)
			c.Mul(&c, &inv)
		}
		a.Set(&b)
		b.Set(&c)
		bs := b.Bytes()
		h.Write(bs[:])
	}
	var e, exp bn254fr.Element
	e.SetUint64(7)
	exp.Exp(e, big.NewInt(1234567))
	eb := exp.Bytes()
	h.Write(eb[:])
	var s bn254fr.Element
	s.SetUint64(16)
	s.Sqrt(&s)
	sb := s.Bytes()
	h.Write(sb[:])
}

// fpBattery is frBattery over the base field Fp (the curve coordinate field).
func fpBattery(h hash.Hash) {
	var a, b, c bn254fp.Element
	a.SetUint64(2)
	b.SetUint64(3)
	for i := 0; i < iterations; i++ {
		c.Mul(&a, &b)
		c.Square(&c)
		c.Add(&c, &a)
		c.Sub(&c, &b)
		c.Double(&c)
		c.Neg(&c)
		if i%50 == 0 {
			var inv bn254fp.Element
			inv.Inverse(&b)
			c.Mul(&c, &inv)
		}
		a.Set(&b)
		b.Set(&c)
		bs := b.Bytes()
		h.Write(bs[:])
	}
	var e, exp bn254fp.Element
	e.SetUint64(7)
	exp.Exp(e, big.NewInt(1234567))
	eb := exp.Bytes()
	h.Write(eb[:])
	var s bn254fp.Element
	s.SetUint64(16)
	s.Sqrt(&s)
	sb := s.Bytes()
	h.Write(sb[:])
}

// mimcNative hashes a fixed preimage with the native (off-circuit) MiMC, which
// is itself a long chain of Fr arithmetic.
func mimcNative(h hash.Hash) {
	m := bn254mimc.NewMiMC()
	var x bn254fr.Element
	x.SetUint64(123456789)
	xb := x.Bytes()
	if _, err := m.Write(xb[:]); err != nil {
		fail("mimc write: %v", err)
	}
	h.Write(m.Sum(nil))
}

// parityCircuit constrains Hash == MiMC(Pre). Its R1CS and solved witness are
// fully determined by the field, so they are backend-sensitive yet
// reproducible.
type parityCircuit struct {
	Pre  frontend.Variable `gnark:",public"`
	Hash frontend.Variable `gnark:",public"`
}

func (c *parityCircuit) Define(api frontend.API) error {
	m, err := mimc.NewMiMC(api)
	if err != nil {
		return err
	}
	m.Write(c.Pre)
	api.AssertIsEqual(c.Hash, m.Sum())
	return nil
}

// r1csDigest compiles parityCircuit and hashes its canonical serialization.
func r1csDigest(h hash.Hash) {
	ccs, err := frontend.Compile(ecc.BN254.ScalarField(), r1cs.NewBuilder, &parityCircuit{})
	if err != nil {
		fail("compile: %v", err)
	}
	var buf bytes.Buffer
	if _, err := ccs.WriteTo(&buf); err != nil {
		fail("r1cs serialize: %v", err)
	}
	h.Write(buf.Bytes())
}

// witnessDigest builds the solving witness for a fixed assignment, hashes its
// serialization, and asserts the constraint system actually solves (exercising
// the prover-side field arithmetic).
func witnessDigest(h hash.Hash) {
	var x bn254fr.Element
	x.SetUint64(123456789)
	xb := x.Bytes()

	m := bn254mimc.NewMiMC()
	if _, err := m.Write(xb[:]); err != nil {
		fail("witness mimc: %v", err)
	}
	hsh := m.Sum(nil)

	assignment := &parityCircuit{Pre: xb[:], Hash: hsh}
	w, err := frontend.NewWitness(assignment, ecc.BN254.ScalarField())
	if err != nil {
		fail("witness: %v", err)
	}
	wb, err := w.MarshalBinary()
	if err != nil {
		fail("witness marshal: %v", err)
	}
	h.Write(wb)

	ccs, err := frontend.Compile(ecc.BN254.ScalarField(), r1cs.NewBuilder, &parityCircuit{})
	if err != nil {
		fail("compile (witness): %v", err)
	}
	if err := ccs.IsSolved(w); err != nil {
		fail("constraint system did not solve: %v", err)
	}
}

func fail(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(2)
}
