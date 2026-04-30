Feature: Chats section list
  In order to keep track of important conversations
  As a jirlab user
  I need to see my pinned Microsoft Teams chats in the Chats section

  Scenario: Pinned chats are displayed in the top table
    Given the MSGraph service returns 3 pinned chats
    When the ChatsSection loads chats
    Then the top table shows 3 rows
    And the first row contains the first chat topic

  Scenario: Empty chats list shows informative message
    Given the MSGraph service returns 0 pinned chats
    When the ChatsSection loads chats
    Then the top table shows a message "No pinned chats"

  Scenario: Tab key switches between top and bottom panes
    Given the ChatsSection is in the top pane
    When the user presses tab
    Then the ChatsSection is in the bottom pane

  Scenario: Tab key switches from bottom back to top pane
    Given the ChatsSection is in the bottom pane
    When the user presses tab
    Then the ChatsSection is in the top pane
