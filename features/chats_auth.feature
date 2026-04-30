Feature: Chats section authentication
  In order to use the Chats section
  As a jirlab user
  I need Microsoft Graph authentication to work correctly

  Scenario: Authenticated user sees chats immediately
    Given the MSGraph token file exists at the token path
    And the MSGraph service returns pinned chats
    When the ChatsSection is initialized
    Then the chats table shows the pinned chats
    And no authentication placeholder is shown

  Scenario: Unauthenticated user sees placeholder row
    Given no MSGraph token file exists
    When the ChatsSection is initialized
    Then the chats table shows a placeholder row "Authentication required"

  Scenario: Device code flow is started on request
    Given no MSGraph token file exists
    And the ChatsSection shows the authentication placeholder
    When the user requests authentication
    Then the device code info is retrieved
    And the verification URL is opened in the browser
    And the user code is copied to the clipboard
