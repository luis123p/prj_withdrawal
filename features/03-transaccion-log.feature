Feature: Transaction integrity
  As the bank (ATM service)
  I want every withdrawal and deposit to be atomic and recorded
  So that accounting stays consistent and can be audited

  Rule: Every attempt is logged in JSON with status EXITOSA, RECHAZADA or ERROR

    Scenario: Successful withdrawal is logged
      Given Alice has 234.56 in their account
      When Alice withdraws 200.00
      Then a transaction "RETIRO" with status "EXITOSA" is logged
      And the logged final balance is 34.56

    Scenario: Declined withdrawal is logged
      Given Hamza has 198.76 in their account
      When Hamza tries to withdraw 200.00
      Then a transaction "RETIRO" with status "RECHAZADA" is logged
      And the balance remains 198.76

    Scenario: Invalid amount is logged as an error
      Given Carla has 150.00 in their account
      When Carla tries to deposit -20.00
      Then a transaction "ABONO" with status "ERROR" is logged
      And the balance remains 150.00

    Scenario: Successful deposit is logged
      Given Bruno has 100.00 in their account
      When Bruno deposits 50.00
      Then a transaction "ABONO" with status "EXITOSA" is logged
      And the logged final balance is 150.00

  Rule: An operation that cannot be logged is rolled back

    Scenario: Log is not available
      Given Alice has 234.56 in their account
      And the transaction log cannot be written
      When Alice withdraws 200.00
      Then the operation is rolled back
      And the balance remains 234.56
