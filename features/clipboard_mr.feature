@clipboard @repos
Feature: Repositories clipboard on MR creation
  In order to quickly share MR links
  As a developer
  I need the MR browser URL copied to clipboard when I create a merge request

  Scenario: MR created copies URL to clipboard
    Given a repo at path "/tmp/test-repo" with branch "feature/PROJ-42-fix-login"
    And GitLab returns MR URL "https://gitlab.example.com/proj/repo/-/merge_requests/7"
    When I create an MR for the current branch
    Then the clipboard contains "https://gitlab.example.com/proj/repo/-/merge_requests/7"

  Scenario: MR creation with no URL does not crash
    Given a repo at path "/tmp/test-repo" with branch "feature/PROJ-42-fix-login"
    And GitLab returns MR URL ""
    When I create an MR for the current branch
    Then the clipboard is not updated
