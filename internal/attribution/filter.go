package attribution

import (
	"github.com/greymass/roborovski/libraries/actionstream"
	"github.com/greymass/roborovski/libraries/chain"
)

const System = "eosio"

var Actions = []string{
	"newaccount",
	"buyram",
	"buyrambytes",
	"buyramself",
	"ramtransfer",
	"giftram",
	"setacctram",
	"logbuyram",
	"ungiftram",
}

func Filter() actionstream.Filter {
	f := actionstream.Filter{Contracts: []uint64{chain.StringToName(System)}}
	for _, a := range Actions {
		f.Actions = append(f.Actions, chain.StringToName(a))
	}
	return f
}
