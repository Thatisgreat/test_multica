package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/ethclient"
)

func main() {
	rpcURL := flag.String("rpc", "https://ethereum-rpc.publicnode.com", "Ethereum RPC endpoint")
	n := flag.Int("n", 10, "number of recent blocks to average")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cli, err := ethclient.DialContext(ctx, *rpcURL)
	if err != nil {
		log.Fatalf("dial rpc: %v", err)
	}
	defer cli.Close()

	latest, err := cli.BlockNumber(ctx)
	if err != nil {
		log.Fatalf("get latest block number: %v", err)
	}
	if latest+1 < uint64(*n) {
		log.Fatalf("chain has only %d blocks, need at least %d", latest+1, *n)
	}

	var sum uint64
	fmt.Printf("Blocks (latest=%d):\n", latest)
	for i := 0; i < *n; i++ {
		num := latest - uint64(i)
		header, err := cli.HeaderByNumber(ctx, new(big.Int).SetUint64(num))
		if err != nil {
			log.Fatalf("get header %d: %v", num, err)
		}
		fmt.Printf("  #%d  gasLimit=%d  gasUsed=%d\n", num, header.GasLimit, header.GasUsed)
		sum += header.GasLimit
	}
	avg := float64(sum) / float64(*n)
	fmt.Printf("\nAverage gasLimit over last %d blocks: %.2f\n", *n, avg)
}
