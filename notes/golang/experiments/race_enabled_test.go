//go:build race

package main

// raceEnabled 在 go test -race 下为 true。
// 用途：跳过读数会被 race runtime 干扰的用例，以及 race detector 会保守报告的 pattern。
const raceEnabled = true
