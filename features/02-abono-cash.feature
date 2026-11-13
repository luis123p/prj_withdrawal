Feature: Depositing cash
  As an authenticated customer
  I want to deposit cash into my account, indicating an amount
  So that my balance increases and I can see my new balance

  Rule: Customers can only deposit positive amounts up to 5000.00 per operation

    Scenario: Successful cash deposit
      Given Alice has 234.56 in their account
      When Alice deposits 100.00 in cash
      Then the deposit is successful
      And the balance is 334.56

    Scenario: Declined deposit over the per-operation limit
      Given Hamza has 198.76 in their account
      When Hamza tries to deposit 6000.00 in cash
      Then the deposit is declined
      And the balance remains 198.76

    Scenario Outline: Invalid deposit amount
      Given Carla has 150.00 in their account
      When Carla tries to deposit <amount> in cash
      Then the system shows the error "el monto a abonar debe ser mayor que cero"
      And the balance remains 150.00

      Examples:
        | amount |
        | 0.00   |
        | -20.00 |

    Scenario: Deposit with more than two decimals
      Given Carla has 150.00 in their account
      When Carla tries to deposit 10.555 in cash
      Then the system shows the error "el monto solo puede tener hasta 2 decimales"
      And the balance remains 150.00
