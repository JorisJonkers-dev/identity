//go:build integration && !race

package web_test

// slowdown is 1 without the race detector: the check is held to the production target.
const slowdown = 1
