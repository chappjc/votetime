// Copyright (c) 2017, Jonathan Chappelow
// Under the ISC license.  See LICENSE.
package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"sort"
	"time"

	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/rpcclient"
)

var host = flag.String("host", "127.0.0.1:9110", "wallet RPC host:port")
var user = flag.String("user", "dcrwallet", "wallet RPC username")
var pass = flag.String("pass", "bananas", "wallet RPC password")
var cert = flag.String("cert", "dcrwallet.cert", "wallet RPC TLS certificate (when notls=false)")
var notls = flag.Bool("notls", false, "Disable use of TLS for wallet connection")

var (
	activeChainParams = &chaincfg.MainNetParams
)

type vote struct {
	voteHash             chainhash.Hash
	voteHeight           int64
	voteTime             time.Time
	ticketPrice          float64
	ticketHash           chainhash.Hash
	ticketHeight         int64
	ticketTime           time.Time
	ticketMaturityHeight int64
	ticketMaturityTime   time.Time
	waitTimeBlocks       int64
	waitTimeSeconds      int64
}

func main() {
	// Parse command line flags
	flag.Parse()

	// Connect to wallet RPC server
	wcl, err := ConnectRPC(*host, *user, *pass, *cert, *notls)
	if err != nil {
		log.Fatalf("Unable to connect to RPC server: %v", err)
	}

	walletInfo, err := wcl.WalletInfo()
	if err != nil {
		log.Fatalf("WalletInfo failed: %v", err)
	}

	log.Println("Wallet connected to node? ", walletInfo.DaemonConnected)

	log.Println("Listing all transaction inputs and outputs...")
	allTxnIOs, err := wcl.ListTransactionsCountFrom("*", 9999999, 0)
	if err != nil {
		log.Fatalf("ListTransactions failed: %v", err)
	}

	// Transactions may occur multiple times in the list, so gather the unique
	// transaction hashes with a map. We could just take the result that is
	// vout[0] of an stakegen, but this easy too.
	knownVotes := make(map[string]bool)
	for _, tx := range allTxnIOs {
		if *tx.TxType == "vote" {
			knownVotes[tx.TxID] = true
		}
	}
	log.Println("Number of votes: ", len(knownVotes))

	waitSeconds := make([]float64, 0, len(knownVotes))
	waitBlocks := make([]int64, 0, len(knownVotes))

	votes := make([]*vote, 0, len(knownVotes))

	for txid := range knownVotes {
		// Get ticket address from previous outpoint of Vin[1] of SSGen
		voteHash, err := chainhash.NewHashFromStr(txid)
		if err != nil {
			log.Printf("Invalid tx hash %s: %v", txid, err)
			continue
		}
		txRaw, err := wcl.GetRawTransaction(voteHash)
		if err != nil {
			log.Printf("GetRawTransaction(vote) failed: %v", err)
			continue
		}

		// Vin[1] spends the stakesubmission of the ticket purchase
		prevout := txRaw.MsgTx().TxIn[1].PreviousOutPoint
		ticketHash := &prevout.Hash
		ticketTxOutIndex := prevout.Index

		// Get block height and time for the vote
		txRawVerbose, err := wcl.GetRawTransactionVerbose(voteHash)
		if err != nil {
			log.Fatalf("GetRawTransactionVerbose(vote) failed: %v", err)
		}
		voteHeight := txRawVerbose.BlockHeight
		voteTime := time.Unix(txRawVerbose.Blocktime, 0)

		// Get block height and time for the ticket
		prevTxRaw, err := wcl.GetRawTransactionVerbose(ticketHash)
		if err != nil {
			log.Fatalf("GetRawTransactionVerbose(ticket) failed: %v", err)
		}

		// Tickets mature 256 blocks after purchase
		ticketPurchaseHeight := prevTxRaw.BlockHeight
		ticketTime := time.Unix(prevTxRaw.Blocktime, 0)
		ticketMaturityHeight := ticketPurchaseHeight + int64(activeChainParams.TicketMaturity)
		// Get time of block at this height
		ticketMaturityBlockHash, _ := wcl.GetBlockHash(ticketMaturityHeight)
		ticketMaturityBlock, _ := wcl.GetBlockHeaderVerbose(ticketMaturityBlockHash)
		ticketMaturityTime := time.Unix(ticketMaturityBlock.Time, 0)

		// Compute time from maturity to vote
		voteWaitBlocks := voteHeight - ticketMaturityHeight
		voteWaitSeconds := voteTime.Sub(ticketMaturityTime)
		//voteWaitDays := voteWaitSeconds.Hours() / 24.0

		ticketPrice := prevTxRaw.Vout[ticketTxOutIndex].Value
		// log.Printf("Ticket %s... (%f DCR) mined in block %d, voted %d blocks (%.2f days) after maturity.",
		// 	prevTxRaw.Txid[:8], ticketPrice, ticketPurchaseHeight, voteWaitBlocks, voteWaitDays)

		votes = append(votes, &vote{
			voteHash:             *voteHash,
			voteHeight:           voteHeight,
			voteTime:             voteTime,
			ticketPrice:          ticketPrice,
			ticketHash:           *ticketHash,
			ticketHeight:         ticketPurchaseHeight,
			ticketTime:           ticketTime,
			ticketMaturityHeight: ticketMaturityHeight,
			ticketMaturityTime:   ticketMaturityTime,
			waitTimeBlocks:       voteWaitBlocks,
			waitTimeSeconds:      int64(voteWaitSeconds.Seconds()),
		})

		waitBlocks = append(waitBlocks, voteWaitBlocks)
		waitSeconds = append(waitSeconds, voteWaitSeconds.Seconds())
	}

	// Compute mean wait time in blocks and seconds
	var avgBlockWait, avgSecondWait float64
	for iv := range waitBlocks {
		avgBlockWait += float64(waitBlocks[iv])
		avgSecondWait += waitSeconds[iv]
	}
	avgBlockWait /= float64(len(waitBlocks))
	avgSecondWait /= float64(len(waitBlocks))
	log.Printf("Mean wait for %d votes: %.1f blocks, %.2f days.", len(knownVotes),
		avgBlockWait, avgSecondWait/86400.0)

	sort.Slice(votes, func(i, j int) bool {
		return votes[i].waitTimeSeconds < votes[j].waitTimeSeconds
	})

	for _, v := range votes {
		log.Printf("Ticket %s... (%f DCR) mined in block %d, voted %d blocks (%.2f days) after maturity.",
			v.ticketHash[:8], v.ticketPrice, v.ticketHeight, v.waitTimeBlocks, v.waitTimeSeconds/86400.0)
	}
}

// ConnectRPC attempts to create a new websocket connection to a legacy RPC
// server with the given credentials.
func ConnectRPC(host, user, pass, cert string, disableTLS bool) (*rpcclient.Client, error) {
	var rpcCerts []byte
	var err error
	if !disableTLS {
		rpcCerts, err = ioutil.ReadFile(cert)
		if err != nil {
			return nil, fmt.Errorf("Failed to read RPC cert file at %s: %s\n",
				cert, err.Error())
		}
		log.Printf("Attempting to connect to RPC server %s as user %s "+
			"using certificate located in %s",
			host, user, cert)
	} else {
		log.Printf("Attempting to connect to RPC server %s as user %s (no TLS)",
			host, user)
	}

	connCfgDaemon := &rpcclient.ConnConfig{
		Host:         host,
		Endpoint:     "ws", // websocket
		User:         user,
		Pass:         pass,
		Certificates: rpcCerts,
		DisableTLS:   disableTLS,
	}

	rpcClient, err := rpcclient.New(connCfgDaemon, nil)
	if err != nil {
		return nil, fmt.Errorf("Failed to start dcrwallet RPC client: %s", err.Error())
	}

	return rpcClient, nil
}
