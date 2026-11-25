package systemviews

import (
	"encoding/json"
	"strconv"

	"bnema.dev/dumber/internal/application/dto"
	"bnema.dev/dumber/internal/domain/entity"
)

// DOMStructureEditor is implemented by DOM adapters that can insert and
// remove elements marked data-sv-anchor without touching their siblings, so
// iframes and inputs elsewhere on the page keep their state.
type DOMStructureEditor interface {
	// InsertAnchorMarkup inserts markup as a child of the parent anchor,
	// before the child anchor before, or at the end when before is empty.
	InsertAnchorMarkup(parent, before, markup string) error
	// RemoveAnchor removes the element of anchor.
	RemoveAnchor(anchor string) error
	// SetAnchorStyleProperty sets a CSS custom property on an anchor.
	SetAnchorStyleProperty(anchor, property, value string) error
}

// Structural anchors of the service groups. They are not regions: region
// swaps replace inner markup, which would re-create every iframe below.
const (
	homeAnchorGroups         = "home-groups"
	homeAnchorGroupPrefix    = "home-group:"
	homeAnchorGridPrefix     = "home-grid:"
	homeAnchorShellPrefix    = "home-svc-shell:"
	homeGroupTitleRegionPref = "home-group-title:"
	homeColumnsProperty      = "--sv-home-columns"
)

func homeGroupAnchor(key string) string      { return homeAnchorGroupPrefix + key }
func homeGridAnchor(key string) string       { return homeAnchorGridPrefix + key }
func homeShellAnchor(id string) string       { return homeAnchorShellPrefix + id }
func homeGroupTitleRegion(key string) string { return homeGroupTitleRegionPref + key }

// homeShape is the structure of the service groups: which groups and
// services exist, in which order, and which slots each service has. Names,
// descriptions, labels and columns are not part of it.
type homeShape struct {
	Groups []homeGroupShape
}

type homeGroupShape struct {
	// Key is a stable page-local identity: groups have no id in home.toml.
	Key      string
	Name     string
	Services []homeServiceShape
}

type homeServiceShape struct {
	ID    string
	Slots string
}

// homeServiceSlots names the slots a service shell holds; a change replaces
// the shell.
func homeServiceSlots(service *entity.HomeService) string {
	slots := ""
	if homeServiceHasAPIWidget(*service) {
		slots += "api,"
	}
	if homeServiceHasActions(*service) {
		slots += "actions,"
	}
	return slots
}

// homeActionsKey identifies the rendered actions of a service.
func homeActionsKey(service *entity.HomeService) string {
	raw, err := json.Marshal(service.Actions)
	if err != nil {
		return ""
	}
	return string(raw)
}

// buildHomeShape returns the shape of groups. Group keys come from old when
// a group matches (see matchHomeGroups); next numbers new keys.
func buildHomeShape(groups []entity.HomeGroup, old homeShape, next *int) homeShape {
	matched := matchHomeGroups(old.Groups, groups)
	shape := homeShape{Groups: make([]homeGroupShape, len(groups))}
	for gi := range groups {
		group := &groups[gi]
		key := ""
		if matched[gi] >= 0 {
			key = old.Groups[matched[gi]].Key
		} else {
			*next++
			key = "g" + strconv.Itoa(*next)
		}
		services := make([]homeServiceShape, len(group.Services))
		for si := range group.Services {
			service := &group.Services[si]
			services[si] = homeServiceShape{ID: service.ID, Slots: homeServiceSlots(service)}
		}
		shape.Groups[gi] = homeGroupShape{Key: key, Name: group.Name, Services: services}
	}
	return shape
}

// matchHomeGroups pairs each new group with an old group index, or -1.
// With as many groups as before, groups pair by position (a rename keeps
// its group). Otherwise a new group pairs with the next unpaired old group,
// in order, that shares a service id, or, when both have no services, the
// same name. Unpaired groups are inserted or removed.
func matchHomeGroups(old []homeGroupShape, groups []entity.HomeGroup) []int {
	matched := make([]int, len(groups))
	if len(old) == len(groups) {
		for i := range matched {
			matched[i] = i
		}
		return matched
	}
	next := 0
	for gi := range groups {
		matched[gi] = -1
		for oi := next; oi < len(old); oi++ {
			if sameHomeGroup(&old[oi], &groups[gi]) {
				matched[gi] = oi
				next = oi + 1
				break
			}
		}
	}
	return matched
}

func sameHomeGroup(old *homeGroupShape, group *entity.HomeGroup) bool {
	if len(old.Services) == 0 && len(group.Services) == 0 {
		return old.Name == group.Name
	}
	for si := range group.Services {
		for _, service := range old.Services {
			if service.ID == group.Services[si].ID {
				return true
			}
		}
	}
	return false
}

// homeStructureOp is one structural DOM edit.
type homeStructureOp struct {
	// Insert into Parent before Before ("" appends). Markup is rendered at
	// apply time; ServiceID or GroupIndex selects what.
	Parent     string
	Before     string
	ServiceID  string
	GroupIndex int
}

// homeStructurePlan is how to move the page from one shape to the next.
type homeStructurePlan struct {
	Removes []string
	// Inserts run in order; each Before anchor exists when it runs.
	Inserts []homeStructureOp
	// Fresh lists the service ids whose shell is new on the page.
	Fresh map[string]bool
}

// planHomeStructure returns the edits from old to next, or false when only
// a remount can do it: a reorder, or ids that are empty or not unique.
func planHomeStructure(old, next homeShape) (homeStructurePlan, bool) {
	oldIDs, ok := homeShapeIDs(old)
	if !ok {
		return homeStructurePlan{}, false
	}
	newIDs, ok := homeShapeIDs(next)
	if !ok {
		return homeStructurePlan{}, false
	}
	// Services kept in place keep their relative order; anything else is a
	// reorder.
	if !sameOrder(filterIDs(oldIDs, newIDs), filterIDs(newIDs, oldIDs)) {
		return homeStructurePlan{}, false
	}
	oldGroups := make(map[string]bool, len(old.Groups))
	for i := range old.Groups {
		oldGroups[old.Groups[i].Key] = true
	}
	plan := homeStructurePlan{Fresh: make(map[string]bool)}
	kept := keptHomeServices(old, next, oldGroups, plan.Fresh)
	plan.Removes = homeRemoves(old, next, kept)
	plan.Inserts = homeInserts(next, oldGroups, kept)
	return plan, true
}

// keptHomeServices returns the services that stay in place: same group,
// same slots. Every other service of next is recorded in fresh.
func keptHomeServices(old, next homeShape, oldGroups, fresh map[string]bool) map[string]bool {
	oldPlace := make(map[string]homePlace)
	for i := range old.Groups {
		for _, service := range old.Groups[i].Services {
			oldPlace[service.ID] = homePlace{Group: old.Groups[i].Key, Slots: service.Slots}
		}
	}
	kept := make(map[string]bool)
	for i := range next.Groups {
		group := &next.Groups[i]
		for _, service := range group.Services {
			place, ok := oldPlace[service.ID]
			if oldGroups[group.Key] && ok && place.Group == group.Key && place.Slots == service.Slots {
				kept[service.ID] = true
				continue
			}
			fresh[service.ID] = true
		}
	}
	return kept
}

// homeRemoves lists the old groups and service shells that go away.
func homeRemoves(old, next homeShape, kept map[string]bool) []string {
	newGroups := make(map[string]bool, len(next.Groups))
	for i := range next.Groups {
		newGroups[next.Groups[i].Key] = true
	}
	var removes []string
	for i := range old.Groups {
		group := &old.Groups[i]
		if !newGroups[group.Key] {
			removes = append(removes, homeGroupAnchor(group.Key))
			continue
		}
		for _, service := range group.Services {
			if !kept[service.ID] {
				removes = append(removes, homeShellAnchor(service.ID))
			}
		}
	}
	return removes
}

// homeInserts lists the new groups and service shells, from the end, so
// every Before anchor is already in place when an insert runs.
func homeInserts(next homeShape, oldGroups, kept map[string]bool) []homeStructureOp {
	var inserts []homeStructureOp
	for gi := len(next.Groups) - 1; gi >= 0; gi-- {
		group := &next.Groups[gi]
		if !oldGroups[group.Key] {
			before := ""
			if gi+1 < len(next.Groups) {
				before = homeGroupAnchor(next.Groups[gi+1].Key)
			}
			inserts = append(inserts, homeStructureOp{Parent: homeAnchorGroups, Before: before, GroupIndex: gi})
			continue
		}
		for si := len(group.Services) - 1; si >= 0; si-- {
			id := group.Services[si].ID
			if kept[id] {
				continue
			}
			before := ""
			if si+1 < len(group.Services) {
				before = homeShellAnchor(group.Services[si+1].ID)
			}
			inserts = append(inserts, homeStructureOp{Parent: homeGridAnchor(group.Key), Before: before, ServiceID: id, GroupIndex: gi})
		}
	}
	return inserts
}

type homePlace struct {
	Group string
	Slots string
}

func homeShapeIDs(shape homeShape) ([]string, bool) {
	var ids []string
	seen := make(map[string]bool)
	for i := range shape.Groups {
		for _, service := range shape.Groups[i].Services {
			if service.ID == "" || seen[service.ID] {
				return nil, false
			}
			seen[service.ID] = true
			ids = append(ids, service.ID)
		}
	}
	return ids, true
}

// filterIDs returns the ids of a that are also in b, in a's order.
func filterIDs(a, b []string) []string {
	in := make(map[string]bool, len(b))
	for _, id := range b {
		in[id] = true
	}
	var out []string
	for _, id := range a {
		if in[id] {
			out = append(out, id)
		}
	}
	return out
}

func sameOrder(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// homeGroupLeaves renders the swappable leaves of the groups: group titles
// and service tiles.
func homeGroupLeaves(load dto.HomeLoadResponse, shape homeShape) map[string]string {
	leaves := make(map[string]string)
	for gi := range load.Layout.Groups {
		group := &load.Layout.Groups[gi]
		leaves[homeGroupTitleRegion(shape.Groups[gi].Key)] = mustRenderComponent(HomeGroupTitle(group.Name))
		for si := range group.Services {
			service := &group.Services[si]
			leaves[homeServiceTileRegion(service.ID)] = mustRenderComponent(HomeServiceTile(*service))
		}
	}
	return leaves
}

// homeActionKeys maps service ids to their actions key.
func homeActionKeys(load dto.HomeLoadResponse) map[string]string {
	keys := make(map[string]string)
	for gi := range load.Layout.Groups {
		group := &load.Layout.Groups[gi]
		for si := range group.Services {
			service := &group.Services[si]
			if homeServiceHasActions(*service) {
				keys[service.ID] = homeActionsKey(service)
			}
		}
	}
	return keys
}

// findHomeServiceAt returns the service id of group gi.
func findHomeServiceAt(load dto.HomeLoadResponse, gi int, id string) (*entity.HomeService, bool) {
	if gi < 0 || gi >= len(load.Layout.Groups) {
		return nil, false
	}
	group := &load.Layout.Groups[gi]
	for si := range group.Services {
		if group.Services[si].ID == id {
			return &group.Services[si], true
		}
	}
	return nil, false
}
