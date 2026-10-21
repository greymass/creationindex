# creationindex

An index of every account creation on an Antelope chain and the RAM behind it, aware of the VP-0001 gift program so it also yields program metrics.

## Language

**Creation**:
One account coming into existence through an `eosio::newaccount` action, together with the transaction that carried it.
_Avoid_: signup, registration, new account

**Creator**:
The account named as `creator` in the `newaccount` action of a creation. Every creation has one.
_Avoid_: parent, sponsor, payer

**Program creator**:
An account the gift program has admitted to gift endowment RAM, named as `creator` in its `giftacct` actions. It need not be the creator of the creation it gifts to.
_Avoid_: operator, approved creator, gifter

**RAM contribution**:
One delivery of RAM to the created account within its creation transaction, with a kind and an amount. RAM that arrives in a later transaction is not part of the creation.
_Avoid_: funding, RAM source, top-up

**Contribution kind**:
What a RAM contribution is: a buy (the payer spends EOS on the RAM market), a transfer (the giver hands over quota it already holds), a gift (a transfer the giver can reclaim), a program gift, or a quota set (the chain's producers set the account's quota directly).
_Avoid_: RAM source, method

**Delivered bytes**:
The RAM quota a contribution actually added to the created account. Unknown for purchases made before 2024-03-16, whose actions state only an EOS amount or a byte request.
_Avoid_: bytes bought, RAM amount

**Requested bytes**:
The byte count a payer asked for in a `buyrambytes` from before 2024-03-16. It runs slightly above the delivered bytes, so it is an estimate and never a delivered figure.
_Avoid_: bytes (unqualified)

**Program gift**:
A gift whose giver is the program account. It is its own contribution kind, and it has a program creator when a `giftacct` sent it.
_Avoid_: grant, endowment gift, subsidy

**Program account**:
The account holding the endowment and running the gift contract: `gift.vaulta` on Vaulta, `gift.gm` on Jungle 4.
_Avoid_: gift contract (for the account), endowment account

**Return**:
An `ungiftram` by which a giftee sends program RAM back to the program account.
_Avoid_: clawback, refund

**Quota window**:
The rolling 24-hour period a program creator's daily quota applies to. It opens at the creator's first gift after the previous window has run its 24 hours, so its boundaries drift away from midnight. Quota use is always reported per quota window.
_Avoid_: day, quota day

**Program-gifted balance**:
The RAM a giftee holds from the program account: every program gift to it since its last return. A return hands back the whole balance.
_Avoid_: gift balance, outstanding gift

**Program event**:
One recorded change in the gift program's history: an admin action on the program account, a program gift, a return, or other RAM moving into or out of the program account.
_Avoid_: program log, program action
