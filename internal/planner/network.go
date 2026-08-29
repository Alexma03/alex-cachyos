package planner

// NetworkGroup collects network-required steps with the same typed operation.
// Groups are created in the order their first step appears in the plan, and
// steps within each group retain plan order.
type NetworkGroup struct {
	Operation Operation
	Steps     []Step
}

// NetworkGroups groups only steps explicitly marked NetworkRequired. The
// description and all other text metadata are deliberately ignored for this
// classification.
func NetworkGroups(plan Plan) []NetworkGroup {
	var groups []NetworkGroup
	indexes := make(map[Operation]int)
	for _, step := range plan.Steps {
		if step.Network != NetworkRequired { continue }
		index, ok := indexes[step.Operation]
		if !ok {
			index = len(groups)
			indexes[step.Operation] = index
			groups = append(groups, NetworkGroup{Operation: step.Operation})
		}
		groups[index].Steps = append(groups[index].Steps, cloneStep(step))
	}
	return groups
}
