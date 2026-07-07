package main

import (
	"fmt"

	"github.com/MohammadAminKoohi/ChainSim/internal/simulation"
)

func main() {
	fmt.Println("=======================================")
	fmt.Println("       ChainSim — Bitcoin Simulator    ")
	fmt.Println("=======================================")

	// simulation.RunExperimentA()
	// simulation.RunExperimentB()
	// simulation.RunExperimentC()
	// simulation.RunExperimentD()
	simulation.RunSelfishMiningExperiment()

	fmt.Println("\n=======================================")
	fmt.Println(" All experiments completed successfully.")
	fmt.Println("=======================================")
}