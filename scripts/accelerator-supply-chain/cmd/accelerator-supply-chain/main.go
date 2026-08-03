package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	supplychain "kubikles/scripts/accelerator-supply-chain"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "accelerator-supply-chain: failed")
		os.Exit(1)
	}
}

func run(arguments []string) error {
	if len(arguments) == 0 {
		return errors.New("missing command")
	}
	switch arguments[0] {
	case "validate-contract":
		if len(arguments) != 2 {
			return errors.New("invalid arguments")
		}
		toolchain, err := supplychain.LoadToolchain(filepath.Join(arguments[1], "security", "accelerator-toolchain.json"))
		if err != nil {
			return err
		}
		return supplychain.ValidateWorkflows(arguments[1], toolchain)
	case "normalize-spdx":
		if len(arguments) != 7 {
			return errors.New("invalid arguments")
		}
		epoch, err := strconv.ParseInt(arguments[6], 10, 64)
		if err != nil {
			return err
		}
		input, err := os.ReadFile(arguments[1])
		if err != nil {
			return err
		}
		output, err := supplychain.NormalizeSPDX(input, arguments[3], arguments[4], arguments[5], epoch)
		if err != nil {
			return err
		}
		return writePrivate(arguments[2], output)
	case "bundle-create":
		if len(arguments) != 7 {
			return errors.New("invalid arguments")
		}
		epoch, err := strconv.ParseInt(arguments[6], 10, 64)
		if err != nil {
			return err
		}
		return supplychain.CreateBundle(arguments[1], arguments[2], arguments[3], arguments[4], arguments[5], epoch)
	case "bundle-verify":
		if len(arguments) != 6 {
			return errors.New("invalid arguments")
		}
		_, err := supplychain.VerifyBundle(arguments[1], arguments[2], arguments[3], arguments[4], arguments[5])
		return err
	case "attestation-plan":
		if len(arguments) != 8 {
			return errors.New("invalid arguments")
		}
		plan, err := supplychain.ExactAttestationPlan(supplychain.ReleaseDigests{BuildVersion: arguments[1], ImageIndex: arguments[2], ImageAMD64: arguments[3], ImageARM64: arguments[4], Chart: arguments[5], Descriptor: arguments[6]})
		if err != nil {
			return err
		}
		encoded, err := json.MarshalIndent(plan, "", "  ")
		if err != nil {
			return err
		}
		return writePrivate(arguments[7], append(encoded, '\n'))
	case "policy":
		if len(arguments) != 5 {
			return errors.New("invalid arguments")
		}
		var report supplychain.ScanReport
		data, err := os.ReadFile(arguments[1])
		if err != nil || json.Unmarshal(data, &report) != nil {
			return errors.New("scan report invalid")
		}
		exceptionData, err := os.ReadFile(arguments[2])
		if err != nil {
			return err
		}
		exceptions, err := supplychain.ParseExceptions(exceptionData)
		if err != nil {
			return err
		}
		now, err := time.Parse(time.RFC3339, arguments[3])
		if err != nil {
			return err
		}
		result, err := supplychain.EvaluatePolicy(report, exceptions, now.UTC(), true)
		if err != nil {
			return err
		}
		encoded, _ := json.Marshal(result)
		if strings.Contains(strings.ToLower(string(encoded)), "token") {
			return errors.New("unsafe result")
		}
		return writePrivate(arguments[4], append(encoded, '\n'))
	default:
		return errors.New("unknown command")
	}
}

func writePrivate(path string, data []byte) error {
	if !filepath.IsAbs(path) {
		return errors.New("output must be absolute")
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return nil
}
