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

  Rule: The ATM only dispenses multiples of S/ 10 up to S/ 2000 per day

    Scenario: Withdrawal that is not a multiple of 10
      Given Alice has 234.56 in their account
      When Alice tries to withdraw 25.00
      Then the system shows the error "el monto a retirar debe ser múltiplo de S/ 10.00"
      And the balance remains 234.56

    Scenario Outline: Withdrawal amounts the ATM cannot dispense
      Given Carla has 150.00 in their account
      When Carla tries to withdraw <amount>
      Then the system shows the error "el monto a retirar debe ser múltiplo de S/ 10.00"
      And the balance remains 150.00

      Examples:
        | amount |
        | 15.00  |
        | 0.50   |
        | 105.00 |

    Scenario: Withdrawal over the daily limit
      Given Diana has 5000.00 in their account
      And Diana has already withdrawn 1900.00 today
      When Diana tries to withdraw 200.00
      Then the system shows the error "excede el límite diario de retiro (S/ 2000.00; disponible hoy: S/ 100.00)"
      And the balance remains 5000.00

    Scenario: Withdrawal that reaches the daily limit exactly
      Given Diana has 5000.00 in their account
      And Diana has already withdrawn 1900.00 today
      When Diana tries to withdraw 100.00
      Then the withdrawal is successful
      And the balance is 4900.00
