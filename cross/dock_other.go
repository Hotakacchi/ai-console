//go:build !darwin

package main

// Dock があるのは Mac だけ (Windows・Linux は、隠すとトレイだけになる)
func setDockVisible(bool) {}
