@clipboard @board
Feature: Sprint Board clipboard on branch creation
  In order to keep ticket context in the repository
  As a developer
  I need the ticket description saved to .jirlab/ticket/<key>.md
  and the file path copied to clipboard when I create a branch

  Scenario: Branch created saves ticket description and copies path
    Given a Jira issue "PROJ-42" with description "Fix the login flow"
    And a repo exists at path "/tmp/test-repo"
    When I create a branch for issue "PROJ-42" in repo "/tmp/test-repo"
    Then a file exists at "/tmp/test-repo/.jirlab/ticket/PROJ-42.md"
    And the file contains "Fix the login flow"
    And the clipboard contains "/tmp/test-repo/.jirlab/ticket/PROJ-42.md"

  Scenario: Branch created with empty description saves placeholder
    Given a Jira issue "PROJ-99" with description ""
    And a repo exists at path "/tmp/test-repo"
    When I create a branch for issue "PROJ-99" in repo "/tmp/test-repo"
    Then a file exists at "/tmp/test-repo/.jirlab/ticket/PROJ-99.md"
    And the clipboard contains "/tmp/test-repo/.jirlab/ticket/PROJ-99.md"
