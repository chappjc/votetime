# votetime

`votetime` is a command line tool to determine the duration of time that your
tickets were live, from maturity to the vote. It will also compute the mean
duration over all of your tickets.

## Installation

    go get -u github.com/chappjc/votetime

If there are errors about a vendor folder in another repository, it is safe to
delete those folders as they are automatically generated.

## Usage

To use `votetime`, you only need to point it your running dcrwallet's RPC
server. Ensure that dcrwallet is running and synchronized to the best block.
Next, set the host and authentication information with command line flags:

```none
Usage of ./votetime:
  -cert string
        wallet RPC TLS certificate (when notls=false) (default "dcrwallet.cert")
  -host string
        wallet RPC host:port (default "127.0.0.1:9110")
  -notls
        Disable use of TLS for wallet connection
  -pass string
        wallet RPC password (default "bananas")
  -user string
        wallet RPC username (default "dcrwallet")
```

For example:

    votetime -user me -pass fluffy -cert ~/.dcrwallet/rpc.cert

If your wallet is running and listening on the network interface and port number
you have specified with the `-host` flag, votetime will begin by listing all of
your wallet's transactions. Next it will make a list of votes (SSGen
transactions) recognized by your wallet. For each vote, it identifies the
corresponding ticket purchase (SSTx) and computes the time elapsed between
ticket maturity (256 blocks after purchase) and redemption (the vote).

## License

`votetime` is distributable under the ISC license.  Please see LICENSE for
details.
