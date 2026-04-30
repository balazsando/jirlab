@clipboard @mrs
Feature: Merge Requests patch file path and clipboard
  In order to work on MR patches efficiently
  As a developer
  I need patch files saved to .jirlab/mr/<ticket>.patch and the path copied to clipboard

  Scenario: Patch saved with ticket key resolved from source branch
    Given an MR with source branch "feature/PROJ-42-fix-login" and project ID 5 and IID 3
    And a local repo for project 5 at path "/tmp/test-repo"
    When I download the patch file
    Then a file is saved at "/tmp/test-repo/.jirlab/mr/PROJ-42.patch"
    And the clipboard contains "/tmp/test-repo/.jirlab/mr/PROJ-42.patch"

  Scenario: Patch saved with MR ID when no ticket key in branch name
    Given an MR with source branch "hotfix-no-ticket" and project ID 5 and IID 7
    And a local repo for project 5 at path "/tmp/test-repo"
    When I download the patch file
    Then a file is saved at "/tmp/test-repo/.jirlab/mr/mr-7.patch"
    And the clipboard contains "/tmp/test-repo/.jirlab/mr/mr-7.patch"

  Scenario: Patch directory is created if it does not exist
    Given an MR with source branch "feature/NEW-1-something" and project ID 5 and IID 9
    And a local repo for project 5 at path "/tmp/test-repo"
    When I download the patch file
    Then a file is saved at "/tmp/test-repo/.jirlab/mr/NEW-1.patch"
