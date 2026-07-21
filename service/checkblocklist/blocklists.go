package checkblocklist

import (
	"errors"
	"fmt"
)

var (
	ErrInvalidBlockListName = errors.New("invalid block list name")
)

type BlockListName string

const (
	BlockListNameSpamHaus    BlockListName = "spamhaus"
	BlockListNameSpamCop     BlockListName = "spamcop"
	BlockListNameBarracuda   BlockListName = "barracuda"
	BlockListNamePSBL        BlockListName = "psbl"
	BlockListNameInvaluement BlockListName = "invaluement"
)

var AllBlockListNames = []BlockListName{
	BlockListNameSpamHaus, BlockListNameSpamCop, BlockListNameBarracuda,
	BlockListNamePSBL, BlockListNameInvaluement,
}

func ParseBlockListName(raw string) (BlockListName, error) {
	switch raw {
	case string(BlockListNameSpamHaus):
		return BlockListNameSpamHaus, nil
	case string(BlockListNameSpamCop):
		return BlockListNameSpamCop, nil
	case string(BlockListNameBarracuda):
		return BlockListNameBarracuda, nil
	case string(BlockListNameInvaluement):
		return BlockListNameInvaluement, nil
	default:
		return "", fmt.Errorf("unexpected block list name %q: %w", raw, ErrInvalidBlockListName)
	}
}

func ParseBlockListNames(raws []string) ([]BlockListName, error) {
	var (
		names []BlockListName
		errs  []error
	)

	for _, raw := range raws {
		name, err := ParseBlockListName(raw)
		if err != nil {
			errs = append(errs, err)
		} else {
			names = append(names, name)
		}
	}

	return names, errors.Join(errs...)
}

type BlockList struct {
	Name      BlockListName
	Domain    string
	DelistURL string
}

var (
	BlockListSpamHaus = BlockList{
		Domain:    "zen.spamhaus.org",
		DelistURL: "https://check.spamhaus.org/listed/?searchterm={ip}",
	}
	BlockListSpamCop = BlockList{
		Domain:    "bl.spamcop.net",
		DelistURL: "https://www.spamcop.net/bl.shtml?ip={ip}",
	}
	BlockListBarracuda = BlockList{
		Domain:    "b.barracudacentral.org",
		DelistURL: "https://barracudacentral.org/lookups/lookup-reputation?search={ip}",
	}
	BlockListPSBL = BlockList{
		Domain:    "psbl.surriel.com",
		DelistURL: "https://psbl.org/",
	}
	BlockListInvaluement = BlockList{
		Domain:    "ivmSIP.dnsbl.invaluement.com",
		DelistURL: "https://www.invaluement.com/removal/",
	}
)

var Lists = map[BlockListName]BlockList{
	BlockListNameSpamHaus:    BlockListSpamHaus,
	BlockListNameSpamCop:     BlockListSpamCop,
	BlockListNameBarracuda:   BlockListBarracuda,
	BlockListNamePSBL:        BlockListPSBL,
	BlockListNameInvaluement: BlockListInvaluement,
}
