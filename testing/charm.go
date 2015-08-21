// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package testing

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/juju/utils/fs"

	"gopkg.in/juju/charm.v5"
	"gopkg.in/juju/charm.v5/charmrepo"
)

func check(err error) {
	if err != nil {
		panic(err)
	}
}

// NewRepo returns a new testing charm repository rooted at the given
// path, relative to the package directory of the calling package, using
// defaultSeries as the default series.
func NewRepo(path, defaultSeries string) *Repo {
	// Find the repo directory. This is only OK to do
	// because this is running in a test context
	// so we know the source is available.
	_, file, _, ok := runtime.Caller(1)
	if !ok {
		panic("cannot get caller")
	}
	r := &Repo{
		path:          filepath.Join(filepath.Dir(file), path),
		defaultSeries: defaultSeries,
	}
	_, err := os.Stat(r.path)
	if err != nil {
		panic(fmt.Errorf("cannot read repository found at %q: %v", r.path, err))
	}
	return r
}

// Repo represents a charm repository used for testing.
type Repo struct {
	path          string
	defaultSeries string
}

func (r *Repo) Path() string {
	return r.path
}

func clone(dst, src string) string {
	dst = filepath.Join(dst, filepath.Base(src))
	check(fs.Copy(src, dst))
	return dst
}

// BundleDirPath returns the path to a bundle directory with the given name in the
// default series
func (r *Repo) BundleDirPath(name string) string {
	return filepath.Join(r.Path(), "bundle", name)
}

// BundleDir returns the actual charm.BundleDir named name.
func (r *Repo) BundleDir(name string) *charm.BundleDir {
	b, err := charm.ReadBundleDir(r.BundleDirPath(name))
	check(err)
	return b
}

// CharmDirPath returns the path to a charm directory with the given name in the
// default series
func (r *Repo) CharmDirPath(name string) string {
	return filepath.Join(r.Path(), r.defaultSeries, name)
}

// CharmDir returns the actual charm.CharmDir named name.
func (r *Repo) CharmDir(name string) *charm.CharmDir {
	ch, err := charm.ReadCharmDir(r.CharmDirPath(name))
	check(err)
	return ch
}

// ClonedDirPath returns the path to a new copy of the default charm directory
// named name.
func (r *Repo) ClonedDirPath(dst, name string) string {
	return clone(dst, r.CharmDirPath(name))
}

// ClonedDirPath returns the path to a new copy of the default bundle directory
// named name.
func (r *Repo) ClonedBundleDirPath(dst, name string) string {
	return clone(dst, r.BundleDirPath(name))
}

// RenamedClonedDirPath returns the path to a new copy of the default
// charm directory named name, renamed to newName.
func (r *Repo) RenamedClonedDirPath(dst, name, newName string) string {
	dstPath := filepath.Join(dst, newName)
	err := fs.Copy(r.CharmDirPath(name), dstPath)
	check(err)
	return dstPath
}

// ClonedDir returns an actual charm.CharmDir based on a new copy of the charm directory
// named name, in the directory dst.
func (r *Repo) ClonedDir(dst, name string) *charm.CharmDir {
	ch, err := charm.ReadCharmDir(r.ClonedDirPath(dst, name))
	check(err)
	return ch
}

// ClonedURL makes a copy of the charm directory. It will create a directory
// with the series name if it does not exist, and then clone the charm named
// name into that directory. The return value is a URL pointing at the local
// charm.
func (r *Repo) ClonedURL(dst, series, name string) *charm.URL {
	dst = filepath.Join(dst, series)
	if err := os.MkdirAll(dst, os.FileMode(0777)); err != nil {
		panic(fmt.Errorf("cannot make destination directory: %v", err))
	}
	clone(dst, r.CharmDirPath(name))
	return &charm.URL{
		Schema:   "local",
		Name:     name,
		Revision: -1,
		Series:   series,
	}
}

// CharmArchivePath returns the path to a new charm archive file
// in the directory dst, created from the charm directory named name.
func (r *Repo) CharmArchivePath(dst, name string) string {
	dir := r.CharmDir(name)
	path := filepath.Join(dst, "archive.charm")
	file, err := os.Create(path)
	check(err)
	defer file.Close()
	check(dir.ArchiveTo(file))
	return path
}

// BundleArchivePath returns the path to a new bundle archive file
// in the directory dst, created from the bundle directory named name.
func (r *Repo) BundleArchivePath(dst, name string) string {
	dir := r.BundleDir(name)
	path := filepath.Join(dst, "archive.bundle")
	file, err := os.Create(path)
	check(err)
	defer file.Close()
	check(dir.ArchiveTo(file))
	return path
}

// CharmArchive returns an actual charm.CharmArchive created from a new
// charm archive file created from the charm directory named name, in
// the directory dst.
func (r *Repo) CharmArchive(dst, name string) *charm.CharmArchive {
	ch, err := charm.ReadCharmArchive(r.CharmArchivePath(dst, name))
	check(err)
	return ch
}

// MockCharmStore implements charm/charmrepo.Interface and is used to isolate
// tests that would otherwise need to hit the real charm store.
type MockCharmStore struct {
	charms map[string]map[int]*charm.CharmArchive

	mu            sync.Mutex // protects the following fields
	authAttrs     string
	testMode      bool
	defaultSeries string
}

func NewMockCharmStore() *MockCharmStore {
	return &MockCharmStore{charms: map[string]map[int]*charm.CharmArchive{}}
}

// SetAuthAttrs overwrites the value returned by AuthAttrs
func (s *MockCharmStore) SetAuthAttrs(auth string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.authAttrs = auth
}

// AuthAttrs returns the AuthAttrs for this charm store.
func (s *MockCharmStore) AuthAttrs() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.authAttrs
}

// WithTestMode returns a repository Interface where testMode is set to value
// passed to this method.
func (s *MockCharmStore) WithTestMode(testMode bool) charmrepo.Interface {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.testMode = testMode
	return s
}

// TestMode returns the test mode setting of this charm store.
func (s *MockCharmStore) TestMode() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.testMode
}

// SetDefaultSeries overwrites the default series for this charm store.
func (s *MockCharmStore) SetDefaultSeries(series string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.defaultSeries = series
}

// DefaultSeries returns the default series for this charm store.
func (s *MockCharmStore) DefaultSeries() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.defaultSeries
}

// Resolve implements charm/charmrepo.Interface.Resolve.
func (s *MockCharmStore) Resolve(ref *charm.Reference) (*charm.URL, error) {
	return ref.URL(s.DefaultSeries())
}

// SetCharm adds and removes charms in s. The affected charm is identified by
// charmURL, which must be revisioned. If archive is nil, the charm will be
// removed; otherwise, it will be stored. It is an error to store a archive
// under a charmURL that does not share its name and revision.
func (s *MockCharmStore) SetCharm(charmURL *charm.URL, archive *charm.CharmArchive) error {
	base := charmURL.WithRevision(-1).String()
	if charmURL.Revision < 0 {
		return fmt.Errorf("bad charm url revision")
	}
	if archive == nil {
		delete(s.charms[base], charmURL.Revision)
		return nil
	}
	archiveRev := archive.Revision()
	archiveName := archive.Meta().Name
	if archiveName != charmURL.Name || archiveRev != charmURL.Revision {
		return fmt.Errorf("charm url %s mismatch with archive %s-%d", charmURL, archiveName, archiveRev)
	}
	if _, found := s.charms[base]; !found {
		s.charms[base] = map[int]*charm.CharmArchive{}
	}
	s.charms[base][charmURL.Revision] = archive
	return nil
}

// interpret extracts from charmURL information relevant to both Latest and
// Get. The returned "base" is always the string representation of the
// unrevisioned part of charmURL; the "rev" wil be taken from the charmURL if
// available, and will otherwise be the revision of the latest charm in the
// store with the same "base".
func (s *MockCharmStore) interpret(charmURL *charm.URL) (base string, rev int) {
	base, rev = charmURL.WithRevision(-1).String(), charmURL.Revision
	if rev == -1 {
		for candidate := range s.charms[base] {
			if candidate > rev {
				rev = candidate
			}
		}
	}
	return
}

// Get implements charm/charmrepo.Interface.Get.
func (s *MockCharmStore) Get(charmURL *charm.URL) (charm.Charm, error) {
	base, rev := s.interpret(charmURL)
	charm, found := s.charms[base][rev]
	if !found {
		return nil, fmt.Errorf("charm not found in mock store: %s", charmURL)
	}
	return charm, nil
}

// GetBundle is only defined for implementing Interface.
func (s *MockCharmStore) GetBundle(curl *charm.URL) (charm.Bundle, error) {
	return nil, errors.New("not implemented")
}

// Latest implements charm/charmrepo.Interface.Latest.
func (s *MockCharmStore) Latest(charmURLs ...*charm.URL) ([]charmrepo.CharmRevision, error) {
	result := make([]charmrepo.CharmRevision, len(charmURLs))
	for i, curl := range charmURLs {
		charmURL := curl.WithRevision(-1)
		base, rev := s.interpret(charmURL)
		if _, found := s.charms[base][rev]; !found {
			result[i].Err = fmt.Errorf("charm not found in mock store: %s", charmURL)
		} else {
			result[i].Revision = rev
		}
	}
	return result, nil
}
