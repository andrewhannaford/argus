//go:build !windows

package main

func runAsServiceIfNeeded(server, token, id string) bool {
	return false
}
