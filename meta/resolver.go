package meta

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// responsiblities:
//
// 1.) resolve expands all "uses" with selected "grouping" which includes
// refinements and augments.  Once this is complete the grouping, augments and
// refinement statements are no longer useful and can be removed from the schema
// tree.
//
// 2.) process imports which triggers whole new, recursive chain of processing. This
// is a form of resolving because imports are really just a way of grouping groupings into
// separate files
func resolve(m *Module) error {
	r := &resolver{
		builder:        &Builder{},
		inProgressUses: make(map[interface{}]HasDataDefinitions),
	}
	if err := r.module(m); err != nil {
		return err
	}
	r.fillInRecursiveDefs()
	return nil
}

type recursiveEntry struct {
	master    HasDataDefinitions
	duplicate HasDataDefinitions
}

type resolver struct {
	builder        *Builder
	inProgressUses map[interface{}]HasDataDefinitions
	recursives     []recursiveEntry
}

func (r *resolver) module(y *Module) error {
	if y.featureSet != nil {
		if err := y.featureSet.Initialize(y); err != nil {
			return err
		}
	}

	// exand all includes
	if len(y.includes) > 0 {
		for _, i := range y.includes {
			if i.loader == nil {
				return errors.New("no module loader defined")
			}
			var err error
			var rev string
			if i.rev != nil {
				rev = i.rev.Ident()
			}
			_, err = i.loader(i.parent, i.subName, rev, i.parent.featureSet, i.loader)
			if err != nil {
				return errors.New(i.subName + " - " + err.Error())
			}
		}
	}

	// expand all imports first because local uses may reference groupings in other files.
	if len(y.imports) > 0 {
		// imports were indexed by module name, but now that we know the
		// prefix, we need to reindex them
		byName := y.imports
		y.imports = make(map[string]*Import, len(byName))
		for _, i := range byName {

			if i.loader == nil {
				return fmt.Errorf("%s - no module loader defined", i.moduleName)
			}
			if i.prefix == "" {
				return fmt.Errorf("%s - prefix required on import", i.moduleName)
			}
			var err error
			var rev string
			if i.rev != nil {
				rev = i.rev.Ident()
			}
			i.module, err = i.loader(nil, i.moduleName, rev, i.parent.featureSet, i.loader)
			if err != nil {
				return fmt.Errorf("%s - %s", i.moduleName, err)
			}

			// recurse
			if err = r.module(i.module); err != nil {
				return err
			}

			// imports were originally added by module name, but now that we know the
			// prefix, we need to re-add them with proper key: prefix
			y.imports[i.Prefix()] = i
		}
	}

	//
	// now we can go into definitions and resolve "uses"
	//
	if err := r.dataDef(y, y.popDataDefinitions()); err != nil {
		return err
	}

	for _, a := range y.Augments() {
		if err := r.expandAugment(a, y); err != nil {
			return err
		}
	}

	for _, d := range y.Deviations() {
		if err := r.applyDeviation(y, d); err != nil {
			return err
		}
	}

	return nil
}

func (r *resolver) applyDeviation(y *Module, d *Deviation) error {
	target := Find(y, d.Ident())
	if target == nil {
		return fmt.Errorf("could not find target for deviation %s", d.Ident())
	}
	if d.NotSupported {
		switch target.(type) {
		case *Rpc:
			actions := target.Parent().(HasActions).Actions()
			delete(actions, target.Ident())
		case *Notification:
			notifs := target.Parent().(HasNotifications).Notifications()
			delete(notifs, target.Ident())
		default:
			hasDDefs := target.Parent().(HasDataDefinitions)
			existing := hasDDefs.popDataDefinitions()
			for _, candidate := range existing {
				if candidate != target {
					hasDDefs.addDataDefinition(candidate)
				}
			}
		}
		return nil
	}
	// 7.20.3.2. in RFC details restrictions on deviations. I assume
	// violations are errors, not silent ignores.
	hasDets, _ := target.(HasDetails)
	hasType, _ := target.(Leafable)
	hasListDets, _ := target.(HasListDetails)
	if d.Add != nil {
		if d.Add.configPtr != nil {
			if hasDets.IsConfigSet() {
				return fmt.Errorf("config already set on %s", d.Ident())
			}
			hasDets.setConfig(*(d.Add).configPtr)
		}
		if d.Add.mandatoryPtr != nil {
			if hasDets.IsMandatorySet() {
				return fmt.Errorf("mandatory already set on %s", d.Ident())
			}
			hasDets.setMandatory(*(d.Add).mandatoryPtr)
		}
		if d.Add.maxElementsPtr != nil {
			if hasListDets.IsMaxElementsSet() {
				return fmt.Errorf("max-elements already set on %s", d.Ident())
			}
			hasListDets.setMaxElements(*(d.Add).maxElementsPtr)
		}
		if d.Add.minElementsPtr != nil {
			if hasListDets.IsMinElementsSet() {
				return fmt.Errorf("min-elements already set on %s", d.Ident())
			}
			hasListDets.setMinElements(*(d.Add).minElementsPtr)
		}
		for _, must := range d.Add.musts {
			target.(HasMusts).addMust(must)
		}
		if d.Add.units != "" {
			if hasType.Units() != "" {
				return fmt.Errorf("units already set on %s", d.Ident())
			}
			hasType.setUnits(d.Add.units)
		}
		if d.Add.defaultVal != nil {
			if hasType.Default() != nil {
				return fmt.Errorf("default already set on %s", d.Ident())
			}
			hasType.setDefault(d.Add.defaultVal)
		}
		for _, unique := range d.Add.unique {
			target.(*List).unique = append(target.(*List).unique, unique)
		}
		for _, must := range d.Add.musts {
			target.(HasMusts).addMust(must)
		}
	}
	if d.Replace != nil {
		if d.Replace.configPtr != nil {
			if !hasDets.IsConfigSet() {
				return fmt.Errorf("config not set on %s", d.Ident())
			}
			hasDets.setConfig(*(d.Replace).configPtr)
		}
		if d.Replace.mandatoryPtr != nil {
			if !hasDets.IsMandatorySet() {
				return fmt.Errorf("mandatory not set on %s", d.Ident())
			}
			hasDets.setMandatory(*(d.Replace).mandatoryPtr)
		}
		if d.Replace.maxElementsPtr != nil {
			if !hasListDets.IsMaxElementsSet() {
				return fmt.Errorf("max-elements not set on %s", d.Ident())
			}
			hasListDets.setMaxElements(*(d.Replace).maxElementsPtr)
		}
		if d.Replace.minElementsPtr != nil {
			if !hasListDets.IsMinElementsSet() {
				return fmt.Errorf("min-elements not set on %s", d.Ident())
			}
			hasListDets.setMinElements(*(d.Replace).minElementsPtr)
		}
		if d.Replace.units != "" {
			if hasType.Units() == "" {
				return fmt.Errorf("units not set on %s", d.Ident())
			}
			hasType.setUnits(d.Replace.units)
		}
		if d.Replace.defaultVal != nil {
			if hasType.Default() == nil {
				return fmt.Errorf("default not set on %s", d.Ident())
			}
			hasType.setDefault(d.Replace.defaultVal)
		}
	}
	if d.Delete != nil {
		if d.Delete.units != "" {
			if hasType.Units() == d.Delete.units {
				return fmt.Errorf("cannot delete units '%s' != '%s' on %s",
					d.Delete.units, hasType.Units(), d.Ident())
			}
			hasType.setUnits("")
		}
		if d.Delete.defaultVal != nil {
			if hasType.Default() == d.Delete.defaultVal {
				return fmt.Errorf("cannot delete units '%s' != '%s' on %s",
					d.Delete.defaultVal, hasType.Default(),
					d.Ident())
			}
			hasType.setDefault(nil)
		}
		for _, unique := range d.Delete.unique {
			found := false
			var uniques [][]string
			for _, candidate := range target.(*List).unique {
				if isArrayStringEqual(unique, candidate) {
					found = true
				} else {
					uniques = append(uniques, candidate)
				}
			}
			if !found {
				return fmt.Errorf("unique entry %s not found on %s",
					strings.Join(unique, " "), d.Ident())
			}
			target.(*List).unique = uniques
		}
		for _, must := range d.Delete.musts {
			found := false
			var musts []*Must
			for _, candidate := range target.(HasMusts).Musts() {
				if candidate.Expression() == must.Expression() {
					found = true
				} else {
					musts = append(musts, candidate)
				}
			}
			if !found {
				return fmt.Errorf("must entry %s not found on %s",
					must.Expression(), d.Ident())
			}
			target.(HasMusts).setMusts(musts)
		}

	}
	return nil
}

func isArrayStringEqual(a []string, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	sort.Strings(a)
	sort.Strings(b)
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// this methods adds data definitions (container, list, leaf...) back into
// their parent but resolves all "uses" while doing so. This operation is
// recursive so the module is the last container to have all it's children
// complete.
//
//	           M
//	         a1  b1
//	       c2      d3
//	order would be:
//	   Enter M, Enter A1, Enter c2, Leave c2, Leave a2, Enter b1,
//	   Enter d3, Leave d3, Leave b1, Leave M
func (r *resolver) dataDef(x HasDataDefinitions, defs []Definition) error {
	for _, def := range defs {
		if more, err := r.addDataDef(x, def); err != nil || !more {
			return err
		}
	}

	// we process actions and notification AFTER containers and lists because
	// actions and notifications might be added as part of resolving uses in
	// datadefs lists
	if hasActions, valid := x.(HasActions); valid {
		for _, a := range hasActions.Actions() {
			if i := a.Input(); i != nil {
				if err := r.dataDef(i, i.popDataDefinitions()); err != nil {
					return err
				}
			}

			if o := a.Output(); o != nil {
				if err := r.dataDef(o, o.popDataDefinitions()); err != nil {
					return err
				}
			}
		}
	}

	if hasNotification, valid := x.(HasNotifications); valid {
		for _, n := range hasNotification.Notifications() {
			if err := r.dataDef(n, n.popDataDefinitions()); err != nil {
				return err
			}
		}
	}

	return nil
}

func (r *resolver) addDataDef(parent HasDataDefinitions, child Definition) (bool, error) {
	if hasIf, valid := child.(HasIfFeatures); valid {
		if on, err := checkFeature(hasIf); err != nil || !on {
			return true, err
		}
	}

	if u, isUses := child.(*Uses); isUses {
		g, err := r.findGrouping(u)
		if err != nil {
			return false, err
		}

		// bug recursive detection was incorrectly picking up rpcs inputs and
		// outputs using same groupings when they were different. there is
		// probably a better way to detect recursive definitions and leverage
		// caching to speed up uses/grouping resolution
		_, isContainer := parent.(*Container)
		if IsList(parent) || isContainer {
			if master, foundInCache := r.inProgressUses[u.schemaId]; foundInCache {
				// resolve this uses later
				r.recursives = append(r.recursives, recursiveEntry{master, parent})
				// fmt.Printf("%s : %s <= %s \n", u.ident, SchemaPath(master), SchemaPath(parent))
				return false, nil
			}

			r.inProgressUses[u.schemaId] = parent
		}

		// resolve all children
		groupDefs := r.cloneDefs(parent, g.DataDefinitions(), u.when)
		err = r.dataDef(parent, groupDefs)
		if err != nil {
			return false, err
		}

		if err := r.applyRefinements(u, parent); err != nil {
			return false, err
		}

		for _, a := range u.augments {
			if err := r.expandAugment(a, parent); err != nil {
				return false, err
			}
		}

		// copy in any actions or notifications unresolved, they will be resolved
		// in caller loop
		for _, a := range g.Actions() {
			hasActions, validActions := parent.(HasActions)
			if !validActions {
				return false, fmt.Errorf("cannot add %s. %s does not allow actions", u.ident, SchemaPath(u))
			}
			hasActions.addAction(a.clone(parent).(*Rpc))
		}
		for _, a := range g.Notifications() {
			hasNotifs, validNotifs := parent.(HasNotifications)
			if !validNotifs {
				return false, fmt.Errorf("cannot add %s. %s does not allow notifications", u.ident, SchemaPath(u))
			}
			hasNotifs.addNotification(a.clone(parent).(*Notification))
		}

		return true, nil
	}

	parent.addDataDefinition(child)

	//
	// recurse into container and lists
	//
	if h, recurse := child.(HasDataDefinitions); recurse {
		if err := r.dataDef(h, h.popDataDefinitions()); err != nil {
			return false, err
		}
	}

	//
	// recurse into choices
	//
	if choice, isChoice := child.(*Choice); isChoice {
		for _, k := range choice.Cases() {
			if err := r.dataDef(k, k.popDataDefinitions()); err != nil {
				return false, err
			}
		}
	}

	return true, nil
}

// copy top-level children into lower-level parent and mark
// lower-level parent as recurisive
func (r *resolver) fillInRecursiveDefs() {
	for _, entry := range r.recursives {
		entry.duplicate.popDataDefinitions()
		entry.duplicate.markRecursive()
		for _, def := range entry.master.DataDefinitions() {
			entry.duplicate.addDataDefinition(def)
		}
	}
}

func (r *resolver) cloneDefs(parent HasDataDefinitions, defs []Definition, when *When) []Definition {
	copy := make([]Definition, len(defs))
	for i, d := range defs {
		copy[i] = d.(cloneable).clone(parent).(Definition)
		if when != nil {
			copy[i].(HasWhen).setWhen(when)
		}
	}
	return copy
}

func (r *resolver) findGrouping(y *Uses) (*Grouping, error) {
	prefix, ident := splitIdent(y.Ident())

	// From RFC
	//   A reference to an unprefixed type or grouping, or one that uses the
	//   prefix of the current module, is resolved by locating the matching
	//   "typedef" or "grouping" statement among the immediate substatements
	//   of each ancestor statement.
	// this means if prefix is local module, then ignore it and follow chain
	searchHeirarcy := (prefix == "")
	var module *Module
	if !searchHeirarcy {
		m, isExternal, err := findModuleAndIsExternal(y, prefix)
		if err != nil {
			return nil, err
		}
		if !isExternal {
			searchHeirarcy = true
		} else {
			module = m
		}
	}

	var found *Grouping
	if searchHeirarcy {
		var p Definition
		p = y
		for p != nil {
			if ptd, ok := p.(HasGroupings); ok {
				if found = ptd.Groupings()[ident]; found != nil {
					return found, nil
				}
			}
			p = p.getOriginalParent()
		}
	} else {
		found = module.Groupings()[ident]
	}

	if found == nil {
		return nil, fmt.Errorf("%s - %s group not found", SchemaPath(y), y.Ident())
	}
	return found, nil
}

func (r *resolver) applyRefinements(u *Uses, parent Definition) error {
	for _, refine := range u.refines {
		if on, err := checkFeature(refine); !on || err != nil {
			return err
		}
		target := Find(parent.(HasDataDefinitions), refine.Ident())
		if target == nil {
			return fmt.Errorf("%s:could not find target for refine %s", SchemaPath(u), refine.Ident())
		}
		if err := r.refine(target, refine); err != nil {
			return err
		}
	}
	return nil
}

func (r *resolver) refine(target Definition, y *Refine) error {
	if y.desc != "" {
		r.builder.Description(target, y.desc)
	}
	if y.ref != "" {
		r.builder.Reference(target, y.ref)
	}
	if y.defaultVal != nil {
		r.builder.Default(target, y.defaultVal)
	}
	if y.configPtr != nil {
		r.builder.Config(target, *y.configPtr)
	}
	if y.mandatoryPtr != nil {
		r.builder.Mandatory(target, *y.mandatoryPtr)
	}
	if y.maxElementsPtr != nil {
		r.builder.MaxElements(target, *y.maxElementsPtr)
	}
	if y.minElementsPtr != nil {
		r.builder.MinElements(target, *y.minElementsPtr)
	}
	if y.unboundedPtr != nil {
		r.builder.UnBounded(target, *y.unboundedPtr)
	}
	for _, m := range y.Musts() {
		h, valid := target.(HasMusts)
		if !valid {
			r.builder.setErr(fmt.Errorf("%T does not support must", target))
		} else {
			h.addMust(m.clone(target).(*Must))
		}
	}
	return r.builder.LastErr
}

func (r *resolver) addChild(parent Meta, child Meta) error {
	if IsAction(parent) {
		parent.(HasActions).addAction(child.(*Rpc))
	} else if IsNotification(parent) {
		parent.(HasNotifications).addNotification(child.(*Notification))
	} else if parentDef, hasDefs := parent.(HasDataDefinitions); hasDefs {
		parentDef.addDataDefinition(child.(Definition))
	} else {
		return fmt.Errorf("%T not a recognizable parent for ", parent)
	}
	return nil
}

func (r *resolver) expandAugment(y *Augment, parent Meta) error {
	if on, err := checkFeature(y); !on || err != nil {
		return err
	}

	// RFC7950 Sec 7.17
	// "The target node MUST be either a container, list, choice, case, input,
	//   output, or notification node."
	target := Find(parent.(HasDataDefinitions), y.ident)
	if target == nil {
		return fmt.Errorf("%s - augment target is not found %s", SchemaPath(y), y.ident)
	}
	copy, valid := target.(cloneable)
	if !valid {
		return fmt.Errorf("%T is not a valid type to augment, does not support cloning", target)
	}

	// expand
	if err := r.addChild(parent, copy.(Meta)); err != nil {
		return err
	}
	for _, d := range y.DataDefinitions() {
		if err := r.addChild(copy.(HasDataDefinitions), d); err != nil {
			return err
		}
	}

	return nil
}
