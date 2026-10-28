package main

import "fmt"

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	fmt.Printf("aws-cost-report %s (%s, %s)\n", version, commit, date)
}
