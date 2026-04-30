@tracker
Feature: Time Tracker unified log entry
  In order to log work without overlapping existing entries
  As a developer using jirlab
  I need the log modal to default to 8h, accept a single digit override,
  and always start the new log at the end of the last existing log for the day

  Background:
    Given today is "2026-04-30"

  Scenario: No existing logs - log starts at 8:00
    Given there are no worklogs for today
    When I calculate the log start hour
    Then the start hour should be 8

  Scenario: Existing log ends at 12 - next log starts at 12
    Given there is a worklog from "08:00" to "12:00" for today
    When I calculate the log start hour
    Then the start hour should be 12

  Scenario: Multiple logs - next log starts after the latest end
    Given there is a worklog from "08:00" to "12:00" for today
    And there is a worklog from "12:00" to "14:00" for today
    When I calculate the log start hour
    Then the start hour should be 14

  Scenario: Single digit input overrides hours
    Given the log input has default value "8"
    When the user types "4"
    Then the input value should be "4"

  Scenario: Second digit overwrites the first
    Given the log input has default value "8"
    When the user types "4"
    And the user types "6"
    Then the input value should be "6"

  Scenario: Non-digit input is ignored
    Given the log input has default value "8"
    When the user types "a"
    Then the input value should be "8"

  Scenario: Log appended after last entry with user-specified hours
    Given there is a worklog from "08:00" to "12:00" for today
    When I request to log 4 hours starting from the latest end
    Then the log should be created from "12:00" to "16:00"

  Scenario: Log would exceed 16:00 - error returned
    Given there is a worklog from "08:00" to "16:00" for today
    When I request to log 4 hours starting from the latest end
    Then an error should indicate the day is already fully logged
