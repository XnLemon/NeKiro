package main

import (
	"log"
	"net/http"
	"os"

	"github.com/Nene7ko/NeKiro/agents/internal/challengeproof"
	runtimeb "github.com/Nene7ko/NeKiro/agents/runtime-b"
)

func main() {
	address, err := runtimeb.ListenAddressFromEnvironment(os.LookupEnv)
	if err != nil {
		log.Fatal(err)
	}
	application, err := challengeproof.NewHandler(runtimeb.NewHTTPHandler(runtimeb.NewHandler()), os.LookupEnv)
	if err != nil {
		log.Fatal("runtime-b challenge proof: ", err)
	}
	if err := http.ListenAndServe(address, application); err != nil {
		log.Fatal(err)
	}
}
