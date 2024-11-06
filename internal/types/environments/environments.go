package environments

type Environment string

const (
	Production  Environment = "production"
	Development Environment = "development"
	Staging     Environment = "staging"
	Test        Environment = "test"
)
