package phases

// BuildModelMenuOptions returns the deterministic list of model IDs that an
// operator can pick from for the given GPU architecture. Models whose
// recipe declares GPUArchAllowed without arch in it are filtered out — for
// example, Kimi-K2.6 (Blackwell-only) is hidden on sm_80/sm_86/sm_89/sm_90.
//
// The order is the same as SupportedModelNames() (alphabetical / ASCII).
// Used by interactive menus in setup; --model flag bypasses this filter
// because recommendConfig() then performs the same arch check and emits a
// clear error if the operator forced an incompatible combination.
func BuildModelMenuOptions(arch string) []string {
	all := SupportedModelNames()
	out := make([]string, 0, len(all))
	for _, id := range all {
		recipe, ok := SupportedModels[id]
		if !ok {
			continue
		}
		if !archAllowedByRecipe(recipe, arch) {
			continue
		}
		out = append(out, id)
	}
	return out
}

// archAllowedByRecipe returns true when arch is in recipe.GPUArchAllowed,
// or when the recipe imposes no architecture restriction (empty slice).
func archAllowedByRecipe(recipe ModelRecipe, arch string) bool {
	if len(recipe.GPUArchAllowed) == 0 {
		return true
	}
	for _, a := range recipe.GPUArchAllowed {
		if a == arch {
			return true
		}
	}
	return false
}
