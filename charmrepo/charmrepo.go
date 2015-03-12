// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmrepo

import (
	"net/http"
	"net/url"

	"gopkg.in/errgo.v1"
	"gopkg.in/juju/charmstore.v4/csclient"

	"gopkg.in/juju/charm.v5-unstable"
)

var errNotImplemented = errgo.Newf("not implemented")

// CharmStore2 is a repository Interface that provides access to the public Juju
// charm store.
type CharmStore2 struct {
	client   *csclient.Client
	testMode bool
}

func NewCharmStore(httpClient *http.Client, visitWebPage func(url *url.URL) error, url string) Interface {
	return &CharmStore2{
		client: csclient.New(csclient.Params{
			URL:          url,
			HTTPClient:   httpClient,
			VisitWebPage: visitWebPage,
		}),
	}
}

func (cs *CharmStore2) Get(curl *charm.URL) (charm.Charm, error) {
	return nil, errNotImplemented
}

func (cs *CharmStore2) Latest(curls ...*charm.URL) ([]CharmRevision, error) {
	return nil, errNotImplemented
}

func (cs *CharmStore2) Resolve(ref *charm.Reference) (*charm.URL, error) {
	return nil, errNotImplemented
}
