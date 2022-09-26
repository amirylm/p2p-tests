package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/amir-blox/p2p-tests/utils"
	"gopkg.in/yaml.v3"
	"os"
	"path"
	"sync"
)

func main() {
	var validators, operators, maxGroups int
	var format, outDir string
	flag.IntVar(&validators, "v", 250, "number of validators")
	flag.IntVar(&operators, "n", 50, "number of operators")
	flag.IntVar(&maxGroups, "g", 25, "maximum number of groups")
	flag.StringVar(&outDir, "out", "./", "output directory")
	flag.StringVar(&format, "format", "yaml", "output format")
	flag.Parse()

	groups := utils.Generate(operators, validators, maxGroups)

	ngroups := len(groups)
	nvalidators := 0
	for _, gr := range groups {
		nvalidators += len(gr)
	}
	validatorsAvg := float64(nvalidators) / float64(ngroups)
	st := stats{
		ValidatorsCount: nvalidators,
		OperatorsCount:  operators,
		GroupsCount:     ngroups,
		AvgInGroup:      validatorsAvg,
	}

	fmt.Println("stats:", st)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		raw, err := json.Marshal(groups)
		if err != nil {
			panic(err)
		}
		err = os.WriteFile(path.Join(outDir, ".clusters.json"), raw, 0755)
		//err = os.WriteFile(path.Join(outDir, ".clusters.yaml"), []byte(strings.ReplaceAll(string(raw), "\"", "")), 0755)
		if err != nil {
			panic(err)
		}
		//f, err := os.OpenFile(path.Join(outDir, ".clusters.yaml"), os.O_RDWR|os.O_CREATE, 0755)
		//if err != nil {
		//	panic(err)
		//}
		//defer f.Close()
		//enc := yaml.NewEncoder(f)
		//defer enc.Close()
		//enc.SetIndent(2)
		//if err := enc.Encode(groups); err != nil {
		//	panic(err)
		//}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		f, err := os.OpenFile(path.Join(outDir, ".stats.yaml"), os.O_RDWR|os.O_CREATE, 0755)
		if err != nil {
			panic(err)
		}
		defer f.Close()
		enc := yaml.NewEncoder(f)
		defer enc.Close()
		enc.SetIndent(2)
		if err := enc.Encode(&st); err != nil {
			panic(err)
		}
	}()

	wg.Wait()
}

type stats struct {
	ValidatorsCount int
	OperatorsCount  int
	GroupsCount     int
	AvgInGroup      float64
}
