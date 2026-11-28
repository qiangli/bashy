package main

import (
	"os"

	"github.com/qiangli/bashy/yoke/internal/adapter"
	"github.com/qiangli/yoke/pkg/offer"
)

func main() {
	os.Exit(adapter.RunCobra(offer.NewOfferCmd(), "bashy offer:", nil))
}
