//go:build !linux

package main

import "errors"

// diskSpace has no portable answer off Linux; host runs report it as a failed check.
func diskSpace(string) (uint64, uint64, error) { return 0, 0, errors.New("not on Linux") }
