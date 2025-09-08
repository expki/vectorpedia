#!/bin/bash
rm -rf static/assets
cd ui
npm i
npm run build
cd ..
if ! command -v gcc &> /dev/null; then
    sudo apt update && sudo apt install gcc -y
fi
mkdir -p build
printf "Go: Building...\n"
GOAMD64=v2 GOOS=linux GOARCH=amd64 CGO_ENABLED=1 go build -o build/vectorpedia .
printf "Go: Building AVX2...\n"
GOAMD64=v3 GOOS=linux GOARCH=amd64 CGO_ENABLED=1 go build -o build/vectorpedia-avx2 .
printf "Go: Building AVX512...\n"
GOAMD64=v4 GOOS=linux GOARCH=amd64 CGO_ENABLED=1 go build -o build/vectorpedia-avx512 .
printf "Build completed.\n"
