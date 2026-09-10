//go:build !race

package main

// raceEnabled 在普通 go test（不带 -race）下为 false。
const raceEnabled = false
