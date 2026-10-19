//go:build integration && race

package web_test

// slowdown is how much the race detector slows the check. CI's integration run uses it, beside
// every other package's tests on two CPUs, so the production target only scales there.
const slowdown = 4
