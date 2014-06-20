package charm

import (
	"fmt"
	"io"
	"io/ioutil"
	"regexp"
	"strconv"
	"strings"

	"github.com/juju/names"
	"launchpad.net/goyaml"
)

// BundleData holds the contents of the bundle.
type BundleData struct {
	// Services holds one entry for each service
	// that the bundle will create, indexed by
	// the service name.
	Services map[string]*ServiceSpec

	// Machines holds one entry for each machine referred to
	// by unit placements. These will be mapped onto actual
	// machines at bundle deployment time.
	// It is an error if a machine is specified but
	// not referred to by a unit placement directive.
	Machines map[string]*MachineSpec

	// Series holds the default series to use when
	// the bundle chooses charms.
	Series string

	// Relations holds a slice of 2-element slices,
	// each specifying a relation between two services.
	// Each two-element slice holds two colon-separated
	// (service, relation) pairs - the relation is made between
	// each.
	Relations [][]string
}

// MachineSpec represents a notional machine that will be mapped
// onto an actual machine at bundle deployment time.
type MachineSpec struct {
	Constraints string
	Annotations map[string]string
}

// ServiceSpec represents a single service that will
// be deployed as part of the bundle.
type ServiceSpec struct {
	// Charm holds the charm URL of the charm to
	// use for the given service.
	Charm string

	// NumUnits holds the number of units of the
	// service that will be deployed.
	NumUnits int

	// To may hold up to NumUnits members with
	// each member specifying a desired placement
	// for the respective unit of the service.
	//
	// In regular-expression-like notation, each
	// element matches the following pattern:
	//
	//      (<containertype>:)?(<unit>|<machine>|new)
	//
	// If containertype is specified, the unit is deployed
	// into a new container of that type, otherwise
	// it will be "hulk-smashed" into the specified location.
	//
	// The second part (after the colon) specifies where
	// the new unit should be placed - it may refer to
	// a unit of another service specified in the bundle,
	// a machine id specified in the machines section,
	// or the special name "new" which specifies a newly
	// created machine.
	//
	// A unit placement may be specified with a service name only,
	// in which case its unit number is assumed to
	// be one more than the unit number of the previous
	// unit in the list with the same service, or zero
	// if there were none.
	//
	// If there are less elements in To than NumUnits,
	// the last element is replicated to fill it. If there
	// are no elements (or To is omitted), "new" is replicated.
	//
	// For example:
	//
	//     wordpress/0 wordpress/1 lxc:0 kvm:new
	//
	//  specifies that the first two units get hulk-smashed
	//  onto the first two units of the wordpress service,
	//  the third unit gets allocated onto an lxc container
	//  on machine 0, and subsequent units get allocated
	//  on kvm containers on new machines.
	//
	// The above example is the same as this:
	//
	//     wordpress wordpress lxc:0 kvm:new
	To []string

	// Options holds the configuration values
	// to apply to the new service. They should
	// be compatible with the charm configuration.
	Options map[string]interface{}

	// Annotations holds any annotations to apply to the
	// service when deployed.
	Annotations map[string]string

	// Constraints holds the default constraints to apply
	// when creating new machines for units of the service.
	// This is ignored for units with explicit placement directives.
	Constraints string
}

// ReadBundleData reads bundle data from the given reader.
// The returned data is not verified - call Verify to ensure
// that it is OK.
func ReadBundleData(r io.Reader) (*BundleData, error) {
	bytes, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	var bd BundleData
	if err := goyaml.Unmarshal(bytes, &bd); err != nil {
		return nil, fmt.Errorf("cannot unmarshal bundle data: %v", err)
	}
	return &bd, nil
}

// VerificationError holds an error generated by BundleData.Verify,
// holding all the verification errors found when verifying.
type VerificationError struct {
	Errors []error
}

func (err *VerificationError) Error() string {
	switch len(err.Errors) {
	case 0:
		return "no verification errors!"
	case 1:
		return err.Errors[0].Error()
	}
	return fmt.Sprintf("%s (and %d more errors)", err.Errors[0], len(err.Errors)-1)
}

type bundleDataVerifier struct {
	bd *BundleData

	// machines holds the reference counts of all machines
	// as referred to by placement directives.
	machineRefCounts map[string]int

	errors []error
	verifyConstraints func(c string) error
}

func (verifier *bundleDataVerifier) addErrorf(f string, a ...interface{}) {
	verifier.addError(fmt.Errorf(f, a...))
}

func (verifier *bundleDataVerifier) addError(err error) {
	verifier.errors = append(verifier.errors, err)
}

func (verifier *bundleDataVerifier) err() error {
	if len(verifier.errors) > 0 {
		return &VerificationError{verifier.errors}
	}
	return nil
}

// Verify verifies that the bundle is internally consistent.
// The verifyConstraints function is called to verify any constraints
// that are found.
//
// It verifies the following:
//
// - All defined machines are referred to by placement directives.
// - All services referred to by placement directives are specified in the bundle.
// - All services referred to by relations are specified in the bundle.
// - All constraints are valid.
//
// If the verification fails, Verify returns a *VerificationError describing
// all the problems found.
func (bd *BundleData) Verify(verifyConstraints func(c string) error) error {
	verifier := &bundleDataVerifier{
		verifyConstraints: verifyConstraints,
		bd:                bd,
		machineRefCounts:  make(map[string]int),
	}
	for id := range bd.Machines {
		verifier.machineRefCounts[id] = 0
	}

	verifier.verifyRelations()
	verifier.verifyServices()
	verifier.verifyMachines()

	for id, count := range verifier.machineRefCounts {
		if count == 0 {
			verifier.addErrorf("machine %q is not referred to by a placement directive", id)
		}
	}
	return verifier.err()
}

var validMachineId = regexp.MustCompile("^" + names.NumberSnippet + "$")

func (verifier *bundleDataVerifier) verifyMachines() {
	for id, m := range verifier.bd.Machines {
		if !validMachineId.MatchString(id) {
			verifier.addErrorf("invalid machine id %q found in machines", id)
		}
		if err := verifier.verifyConstraints(m.Constraints); err != nil {
			verifier.addErrorf("invalid constraints in machine %q: %v", id, err)
		}
	}
}

func (verifier *bundleDataVerifier) verifyServices() {
	for name, svc := range verifier.bd.Services {
		if svc.NumUnits < 0 {
			verifier.addErrorf("negative number of units specified on service %q", name)
		}
		if _, err := ParseURL(svc.Charm); err != nil {
			verifier.addErrorf("invalid charm URL in service %q: %v", name, err)
		}
		if err := verifier.verifyConstraints(svc.Constraints); err != nil {
			verifier.addErrorf("invalid constraints in service %q: %v", name, err)
		}
		if len(svc.To) > svc.NumUnits {
			verifier.addErrorf("too many units specified in unit placement for service %q", name)
		}
		verifier.verifyPlacement(svc.To)
	}
}

func (verifier *bundleDataVerifier) verifyPlacement(to []string) {
	for _, p := range to {
		up, err := ParsePlacement(p)
		if err != nil {
			verifier.addError(err)
			continue
		}
		switch {
		case up.Service != "":
			spec, ok := verifier.bd.Services[up.Service]
			if !ok {
				verifier.addErrorf("placement %q refers to non-existent service", p)
				continue
			}
			if up.Unit >= 0 && up.Unit >= spec.NumUnits-1 {
				verifier.addErrorf("placement %q specifies a unit greater than the %d unit(s) started by the target service", p, spec.NumUnits)
			}
		case up.Machine == "new":
		default:
			_, ok := verifier.bd.Machines[up.Machine]
			if !ok {
				verifier.addErrorf("placement %q refers to non-existent machine", p)
				continue
			}
			verifier.machineRefCounts[up.Machine]++
		}
	}
}

type UnitPlacement struct {
	// ContainerType holds the container type of the new
	// new unit, or empty if unspecified.
	ContainerType string

	// Machine holds the numeric machine id, or "new",
	// or empty if the placement specifies a service.
	Machine string

	// Service holds the service name, or empty if
	// the placement specifies a machine.
	Service string

	// Unit holds the unit number of the service, or -1
	// if unspecified.
	Unit int
}

var snippetReplacer = strings.NewReplacer(
	"container", names.ContainerTypeSnippet,
	"number", names.NumberSnippet,
	"service", names.ServiceSnippet,
)

var validPlacement = regexp.MustCompile(
	snippetReplacer.Replace(
		"^(?:(container):)?(?:(service(/number)?)|(number))$",
	),
)

// ParsePlacement parses a unit placement directive, as
// specified in the To clause of a service entry in the
// services section of a bundle.
func ParsePlacement(p string) (*UnitPlacement, error) {
	m := validPlacement.FindStringSubmatch(p)
	if m == nil {
		return nil, fmt.Errorf("invalid placement syntax %q", p)
	}
	up := UnitPlacement{
		ContainerType: m[1],
		Service:       m[2],
		Machine:       m[4],
	}
	if unitStr := m[3]; unitStr != "" {
		// We know that unitStr must be a valid integer because
		// it's specified as such in the regexp.
		up.Unit, _ = strconv.Atoi(unitStr)
	} else {
		up.Unit = -1
	}
	if up.Service == "new" {
		if up.Unit != -1 {
			return nil, fmt.Errorf("invalid placement syntax %q", p)
		}
		up.Machine, up.Service = "new", ""
	}
	return &up, nil
}

func (verifier *bundleDataVerifier) verifyRelations() {
	for i, relPair := range verifier.bd.Relations {
		if len(relPair) != 2 {
			verifier.addErrorf("relation %d has %d relations, not 2", i, len(relPair))
		}
		var svcPair [2]string
		for i, svcRel := range relPair {
			svc, _, err := parseRelation(svcRel)
			if err != nil {
				verifier.addError(err)
			}
			if _, ok := verifier.bd.Services[svc]; !ok {
				verifier.addErrorf("service %q not defined (referred to by relation %q)", svc, relPair)
			}
			svcPair[i] = svc
		}
		if svcPair[0] == svcPair[1] {
			verifier.addErrorf("relation %q relates a service to itself", relPair)
		}
	}
}

var validServiceRelation = regexp.MustCompile("^(" + names.ServiceSnippet + "):(" + names.RelationSnippet + ")$")

func parseRelation(svcRel string) (svc, rel string, err error) {
	m := validServiceRelation.FindStringSubmatch(svcRel)
	if m == nil {
		return "", "", fmt.Errorf("invalid relation syntax %q", svcRel)
	}
	return m[1], m[2], nil
}
