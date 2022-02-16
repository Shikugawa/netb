package network

import (
	"fmt"

	"github.com/Shikugawa/ayame/pkg/config"
	log "github.com/sirupsen/logrus"
	"go.uber.org/multierr"
)

type DirectLink struct {
	VethPair `json:"veth_pair"`
	Name     string `json:"name"`
	Busy     bool   `json:"busy"`
}

func InitDirectLink(cfg *config.LinkConfig) (*DirectLink, error) {
	if cfg.LinkMode != config.ModeDirectLink {
		return nil, fmt.Errorf("invalid mode")
	}

	conf := VethConfig{
		Name: cfg.Name,
	}

	pair, err := InitVethPair(conf)
	if err != nil {
		return nil, err
	}

	return &DirectLink{
		VethPair: *pair,
		Name:     cfg.Name,
		Busy:     false,
	}, nil
}

// TODO: consider error handling
func (d *DirectLink) Destroy() error {
	if !d.Busy {
		return fmt.Errorf("%s is not busy\n", d.Name)
	}

	return d.VethPair.Destroy()
}

// TODO: consider error handling
func (d *DirectLink) CreateLink(left *Namespace, right *Namespace) error {
	if d.Busy {
		return fmt.Errorf("%s has been already busy\n", d.Name)
	}

	if err := (*left).Attach(&d.VethPair.Left); err != nil {
		return err
	}

	if err := (*right).Attach(&d.VethPair.Right); err != nil {
		// TODO: add error handling if left succeeded but right failed.
		return err
	}

	d.Busy = true
	return nil
}

func InitDirectLinks(links []*config.LinkConfig) []*DirectLink {
	var dlinks []*DirectLink
	for _, link := range links {
		if link.LinkMode != config.ModeDirectLink {
			continue
		}

		dlink, err := InitDirectLink(link)
		if err != nil {
			log.Errorf("failed to init direct link: %s", link.Name)
			continue
		}

		dlinks = append(dlinks, dlink)
	}

	return dlinks
}

func CleanupDirectLinks(links []*DirectLink) error {
	var allerr error
	for _, link := range links {
		if err := link.Destroy(); err != nil {
			allerr = multierr.Append(allerr, err)
		}
	}
	return allerr
}
