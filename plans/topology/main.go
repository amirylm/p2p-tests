package main

import "github.com/testground/sdk-go/run"

var testcases = map[string]interface{}{
	"groups": runGroups,
	"subnets": runSubnets,
}

func main() {
	run.InvokeMap(testcases)
}
