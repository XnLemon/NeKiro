package main

import (
	"log"
	"net/http"
	"os"

	"github.com/Nene7ko/NeKiro/agents/internal/challengeproof"
	runtimea "github.com/Nene7ko/NeKiro/agents/runtime-a"
)

func main() {
	config, err := runtimea.LoadConfig(os.LookupEnv)
	if err != nil {
		log.Fatal(err)
	}
	handler, err := runtimea.NewHandler(config, http.DefaultClient)
	if err != nil {
		log.Fatal("runtime-a initialize: ", err)
	}
	application, err := challengeproof.NewHandler(runtimea.NewHTTPHandler(handler), os.LookupEnv)
	if err != nil {
		log.Fatal("runtime-a challenge proof: ", err)
	}
	if err := http.ListenAndServe(config.ListenAddress, application); err != nil {
		log.Fatal("runtime-a serve: ", err)
	}
}
