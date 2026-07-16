package main

import (
	"log"
	"net/http"
	"os"

	runtimeb "github.com/Nene7ko/NeKiro/agents/runtime-b"
)

func main() {
	address, err := runtimeb.ListenAddressFromEnvironment(os.LookupEnv)
	if err != nil {
		log.Fatal(err)
	}
	if err := http.ListenAndServe(address, runtimeb.NewHTTPHandler(runtimeb.NewHandler())); err != nil {
		log.Fatal(err)
	}
}
