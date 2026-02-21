package prover

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark/backend/groth16"
	"github.com/consensys/gnark/constraint"
)

// SaveTo persists a compiled circuit's constraint system and keys to dir.
// Creates dir if it does not exist. Files written:
//
//	circuit.r1cs    — constraint system
//	proving.key     — proving key
//	verifying.key   — verifying key
//	circuit.hash    — SHA-256 of the constraint system (hex)
func (cc *CompiledCircuit) SaveTo(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create key dir: %w", err)
	}

	if err := writeFile(filepath.Join(dir, "circuit.r1cs"), cc.CS); err != nil {
		return fmt.Errorf("save constraint system: %w", err)
	}
	if err := writeFile(filepath.Join(dir, "proving.key"), cc.ProvingKey); err != nil {
		return fmt.Errorf("save proving key: %w", err)
	}
	if err := writeFile(filepath.Join(dir, "verifying.key"), cc.VerifyingKey); err != nil {
		return fmt.Errorf("save verifying key: %w", err)
	}

	hash, err := hashConstraintSystem(cc.CS)
	if err != nil {
		return fmt.Errorf("hash constraint system: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "circuit.hash"), []byte(hash), 0o644); err != nil {
		return fmt.Errorf("save circuit hash: %w", err)
	}

	return nil
}

// LoadFrom loads a compiled circuit from dir. The curve must match what was
// used during setup (typically ecc.BN254 for Ethereum).
func LoadFrom(dir string, curve ecc.ID) (*CompiledCircuit, error) {
	cs := groth16.NewCS(curve)
	if err := readFile(filepath.Join(dir, "circuit.r1cs"), cs); err != nil {
		return nil, fmt.Errorf("load constraint system: %w", err)
	}

	pk := groth16.NewProvingKey(curve)
	if err := readFile(filepath.Join(dir, "proving.key"), pk); err != nil {
		return nil, fmt.Errorf("load proving key: %w", err)
	}

	vk := groth16.NewVerifyingKey(curve)
	if err := readFile(filepath.Join(dir, "verifying.key"), vk); err != nil {
		return nil, fmt.Errorf("load verifying key: %w", err)
	}

	return &CompiledCircuit{
		CS:           cs,
		ProvingKey:   pk,
		VerifyingKey: vk,
		Constraints:  cs.GetNbConstraints(),
		PublicVars:   cs.GetNbPublicVariables(),
		PrivateVars:  cs.GetNbSecretVariables(),
	}, nil
}

func writeFile(path string, src io.WriterTo) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = src.WriteTo(f)
	return err
}

func readFile(path string, dst io.ReaderFrom) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = dst.ReadFrom(f)
	return err
}

// hashConstraintSystem returns a hex-encoded SHA-256 hash of the serialized
// constraint system. Used for cache invalidation when the circuit changes.
func hashConstraintSystem(cs constraint.ConstraintSystem) (string, error) {
	h := sha256.New()
	if _, err := cs.WriteTo(h); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}
