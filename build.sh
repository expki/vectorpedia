#!/bin/bash

if ! command -v gcc &> /dev/null; then
    sudo apt update && sudo apt install gcc -y
fi
mkdir -p build
printf "Go: Building...\n"
GOAMD64=v2 GOOS=linux GOARCH=amd64 CGO_ENABLED=1 go build -o build/vectorpedia .
printf "Go: Building AVX512...\n"
GOAMD64=v4 GOOS=linux GOARCH=amd64 CGO_ENABLED=1 go build -o build/vectorpedia-avx512 .
printf "Gonum: Building...\n"
GOAMD64=v2 GOOS=linux GOARCH=amd64 CGO_ENABLED=1 go build -tags="gonum" -o build/vectorpedia-gonum .
printf "Gonum: Building AVX512...\n"
GOAMD64=v4 GOOS=linux GOARCH=amd64 CGO_ENABLED=1 go build -tags="gonum" -o build/vectorpedia-gonum-avx512 .
printf "Gorgonia: Building...\n"
GOAMD64=v2 GOOS=linux GOARCH=amd64 CGO_ENABLED=1 go build -tags="gorgonia" -o build/vectorpedia-gorgonia .
printf "Gorgonia: Building AVX512...\n"
GOAMD64=v4 GOOS=linux GOARCH=amd64 CGO_ENABLED=1 go build -tags="gorgonia avx" -o build/vectorpedia-gorgonia-avx512 .
printf "Build completed.\n"
