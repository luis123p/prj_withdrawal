Feature: Withdrawing cash
  As an authenticated customer
  I want to withdraw cash, indicating an amount
  So that I can use my savings and see my new balance

  Rule: Customers cannot withdraw more than their balance

    Scenario: Successful withdrawal within balance
      Given Alice has 234.56 in their account
      When Alice tries to withdraw 200.00
      Then the withdrawal is successful
      And the balance is 34.56

    Scenario: Declined withdrawal in excess of balance
      Given Hamza has 198.76 in their account
      When Hamza tries to withdraw 200.00
      Then the withdrawal is declined
      And the balance remains 198.76

  Rule: The amount must be valid

    Scenario Outline: Withdrawal with an invalid amount
      Given Bruno has 100.00 in their account
      When Bruno tries to withdraw <amount>
      Then the system shows the error "el monto a retirar debe ser mayor que cero"
      And the balance remains 100.00

      Examples:
        | amount |
        | -50.00 |
        | 0.00   |
