Feature: Graduated Verification — derived contracts for small features and configurable attempt cap

  @requirement:req-derived-small @requirement:req-derived-coverage @requirement:req-derived-bdd @requirement:req-configurable-attempts @requirement:req-attempt-cap-enforced @requirement:req-amend-resets @requirement:req-backward-compat
  @criterion:ac-derived-from-graph @criterion:ac-derived-coverage @criterion:ac-max-attempts-1-blocks @criterion:ac-max-attempts-4-allows @criterion:ac-default-2 @criterion:ac-amend-resets
  @surface:surface-derived-put @surface:surface-derivation-engine @surface:surface-domain-attempt-cap @surface:surface-workflow-cap
  @judgment:j-bdd-scenarios
  @scenario:scenario-small-feature-derives-correct-contract
  Scenario: Small feature derives correct contract from work graph
    Given a feature design with complexity "small"
    And a work graph with items carrying validation_commands
    When the operator runs "hermoso verification put --derive <work-graph>"
    Then the persisted contract contains one exit_code judgment per validation_commands entry

  @requirement:req-derived-small @requirement:req-derived-coverage @requirement:req-derived-bdd @requirement:req-configurable-attempts @requirement:req-attempt-cap-enforced @requirement:req-amend-resets @requirement:req-backward-compat
  @criterion:ac-derived-from-graph @criterion:ac-derived-coverage @criterion:ac-max-attempts-1-blocks @criterion:ac-max-attempts-4-allows @criterion:ac-default-2 @criterion:ac-amend-resets
  @surface:surface-derived-put @surface:surface-derivation-engine @surface:surface-domain-attempt-cap @surface:surface-workflow-cap
  @judgment:j-bdd-scenarios
  @scenario:scenario-standard-feature-rejects-derive
  Scenario: Standard feature rejects --derive
    Given a feature design with complexity "standard"
    When the operator runs "hermoso verification put --derive <work-graph>"
    Then the CLI exits non-zero

  @requirement:req-derived-small @requirement:req-derived-coverage @requirement:req-derived-bdd @requirement:req-configurable-attempts @requirement:req-attempt-cap-enforced @requirement:req-amend-resets @requirement:req-backward-compat
  @criterion:ac-derived-from-graph @criterion:ac-derived-coverage @criterion:ac-max-attempts-1-blocks @criterion:ac-max-attempts-4-allows @criterion:ac-default-2 @criterion:ac-amend-resets
  @surface:surface-derived-put @surface:surface-derivation-engine @surface:surface-domain-attempt-cap @surface:surface-workflow-cap
  @judgment:j-bdd-scenarios
  @scenario:scenario-derivation-covers-all-criteria
  Scenario: Derivation covers all acceptance criteria
    Given a small feature design with requirements and acceptance criteria
    And a work graph whose items reference those criteria
    When the operator runs "hermoso verification put --derive <work-graph>"
    Then the derived contract passes ValidateContract

  @requirement:req-derived-small @requirement:req-derived-coverage @requirement:req-derived-bdd @requirement:req-configurable-attempts @requirement:req-attempt-cap-enforced @requirement:req-amend-resets @requirement:req-backward-compat
  @criterion:ac-derived-from-graph @criterion:ac-derived-coverage @criterion:ac-max-attempts-1-blocks @criterion:ac-max-attempts-4-allows @criterion:ac-default-2 @criterion:ac-amend-resets
  @surface:surface-derived-put @surface:surface-derivation-engine @surface:surface-domain-attempt-cap @surface:surface-workflow-cap
  @judgment:j-bdd-scenarios
  @scenario:scenario-max-one-attempt-blocks-after-failure
  Scenario: max_verification_attempts=1 blocks after failure
    Given a contract with max_verification_attempts set to 1
    When the first verification attempt fails
    Then the run transitions to verification/blocked

  @requirement:req-derived-small @requirement:req-derived-coverage @requirement:req-derived-bdd @requirement:req-configurable-attempts @requirement:req-attempt-cap-enforced @requirement:req-amend-resets @requirement:req-backward-compat
  @criterion:ac-derived-from-graph @criterion:ac-derived-coverage @criterion:ac-max-attempts-1-blocks @criterion:ac-max-attempts-4-allows @criterion:ac-default-2 @criterion:ac-amend-resets
  @surface:surface-derived-put @surface:surface-derivation-engine @surface:surface-domain-attempt-cap @surface:surface-workflow-cap
  @judgment:j-bdd-scenarios
  @scenario:scenario-max-four-attempts-allows-remediation
  Scenario: max_verification_attempts=4 allows up to four attempts
    Given a contract with max_verification_attempts set to 4
    When three verification attempts each fail with remediation between them
    Then the fourth attempt is allowed

  @requirement:req-derived-small @requirement:req-derived-coverage @requirement:req-derived-bdd @requirement:req-configurable-attempts @requirement:req-attempt-cap-enforced @requirement:req-amend-resets @requirement:req-backward-compat
  @criterion:ac-derived-from-graph @criterion:ac-derived-coverage @criterion:ac-max-attempts-1-blocks @criterion:ac-max-attempts-4-allows @criterion:ac-default-2 @criterion:ac-amend-resets
  @surface:surface-derived-put @surface:surface-derivation-engine @surface:surface-domain-attempt-cap @surface:surface-workflow-cap
  @judgment:j-bdd-scenarios
  @scenario:scenario-absent-max-defaults-to-two
  Scenario: Absent max_verification_attempts defaults to two
    Given a contract without max_verification_attempts
    Then RunVerification allows exactly 2 attempts

  @requirement:req-derived-small @requirement:req-derived-coverage @requirement:req-derived-bdd @requirement:req-configurable-attempts @requirement:req-attempt-cap-enforced @requirement:req-amend-resets @requirement:req-backward-compat
  @criterion:ac-derived-from-graph @criterion:ac-derived-coverage @criterion:ac-max-attempts-1-blocks @criterion:ac-max-attempts-4-allows @criterion:ac-default-2 @criterion:ac-amend-resets
  @surface:surface-derived-put @surface:surface-derivation-engine @surface:surface-domain-attempt-cap @surface:surface-workflow-cap
  @judgment:j-bdd-scenarios
  @scenario:scenario-amend-resets-attempt-budget
  Scenario: Amendment resets the attempt budget
    Given a run with a failed verification attempt
    When the verification contract is amended
    Then all prior verification attempts are cleared

  @requirement:req-derived-small @requirement:req-derived-coverage @requirement:req-derived-bdd @requirement:req-configurable-attempts @requirement:req-attempt-cap-enforced @requirement:req-amend-resets @requirement:req-backward-compat
  @criterion:ac-derived-from-graph @criterion:ac-derived-coverage @criterion:ac-max-attempts-1-blocks @criterion:ac-max-attempts-4-allows @criterion:ac-default-2 @criterion:ac-amend-resets
  @surface:surface-derived-put @surface:surface-derivation-engine @surface:surface-domain-attempt-cap @surface:surface-workflow-cap
  @judgment:j-bdd-scenarios
  @scenario:scenario-constitution-contains-small-clause
  Scenario: Constitution contains small-feature derivation clause
    Given the hermoso constitution at docs/constitution.md
    Then Principle II includes text allowing small features to derive verification from work-graph validation commands

  @requirement:req-derived-small @requirement:req-derived-coverage @requirement:req-derived-bdd @requirement:req-configurable-attempts @requirement:req-attempt-cap-enforced @requirement:req-amend-resets @requirement:req-backward-compat
  @criterion:ac-derived-from-graph @criterion:ac-derived-coverage @criterion:ac-max-attempts-1-blocks @criterion:ac-max-attempts-4-allows @criterion:ac-default-2 @criterion:ac-amend-resets
  @surface:surface-derived-put @surface:surface-derivation-engine @surface:surface-domain-attempt-cap @surface:surface-workflow-cap
  @judgment:j-bdd-scenarios
  @scenario:scenario-small-feature-still-publishes-gherkin
  Scenario: Small feature still publishes Gherkin
    Given a small feature with a derived contract
    When the verification run passes
    Then the Gherkin scenarios are published to features/