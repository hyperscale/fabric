provider "test" {
  name = "test_provider"
  port = "not_a_number"

  # Unknown attribute: gohcl rejects it, which is what makes this fixture a
  # ErrProviderInvalid case rather than a ErrProviderNotFound one.
  typo_attribute = true
}
