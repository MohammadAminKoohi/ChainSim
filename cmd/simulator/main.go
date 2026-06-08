package main

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/MohammadAminKoohi/ChainSim/internal/simulation"
)

func main() {
	rand.Seed(time.Now().UnixNano())

	fmt.Println("=======================================")
	fmt.Println("         Block Chain Simulator          ")
	fmt.Println("=======================================")

	simulation.RunExperimentA()
	simulation.RunExperimentB()
	simulation.RunExperimentC()
	simulation.RunExperimentD()

	fmt.Println("\n=======================================")
	fmt.Println(" All experiments completed successfully.")
	fmt.Println("=======================================")
}