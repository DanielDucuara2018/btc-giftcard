# BTC Gift Card Service - Implementation Roadmap

**Last Updated:** February 16, 2026  
**Status:** Planning Phase - Cost Optimization & Lightning Migration & Automation Research

---

## Vision

> **From gift card to payment instrument.** We're not just building a BTC gift card — we're building a Lightning-native payment network. Today: buy a card, redeem BTC. Tomorrow: spend your card balance directly at merchants, online stores, and point-of-sale terminals — all powered by Lightning Network instant payments.

**Short-term (Months 1-4):** Gift card service with Lightning-first redemption  
**Medium-term (Months 5-8):** Direct merchant payments — spend card balance without redemption  
**Long-term (Year 2+):** Payment ecosystem — virtual cards, NFC payments, merchant network

---

## Executive Summary

This roadmap outlines the implementation plan to transform our MVP into a production-ready, cost-optimized BTC gift card service that evolves into a **Lightning-powered payment platform**. Key improvements include:

- **Cost Reduction:** €2,485 → €637 per 1000 cards (74% reduction)
- **Profit Increase:** €2,515 → €4,363 per 1000 cards (73% increase)
- **Processing Speed:** 30-60 minutes → 1 second (Lightning Network)
- **Automation:** Manual processes → Automated reconciliation & funding
- **Future:** Gift cards become spendable at merchants (Lightning payments)

---

## Current Status (Completed ✅)

- ✅ **Phase 1:** Exchange price providers (Coinbase, CoinGecko, Bitstamp) - 27 tests passing
- ✅ **Phase 2:** Message queue system (Redis Streams) - 27 tests passing
- ✅ **Phase 3:** Card service with async queue integration - 9 tests passing
- ✅ **Phase 4:** Worker skeleton with comprehensive TODOs
- ✅ **Documentation:** README, API docs, architecture diagrams
- ✅ **Infrastructure:** PostgreSQL, Redis, basic Go services

**Total Tests Passing:** 81

---

## Phase 1: MVP Launch (On-Chain) - Weeks 1-2

**Goal:** Launch minimal viable product with on-chain Bitcoin only

### 1.1 Payment Integration - Direct Bank Transfers

**Priority:** HIGH  
**Cost Impact:** €0 vs €250 (Stripe fees for 1000 cards)

- [ ] **Set up business bank account**
  - **Primary recommendation:** Qonto (French-regulated, API on all plans, SEPA instant by default)
  - **Alternative:** Revolut Business Company plan (full API access, `POST /pay` endpoint)
  - **Backup:** Wise Business (multi-currency, low FX fees, SEPA + SWIFT support)
  - See **Decision Point #2** and **Appendix A** for full comparison
  - Enable SEPA instant transfers
  - Configure webhook/callback notifications
  - Document account details for customer instructions

- [ ] **Implement semi-manual bank transfer reconciliation**
  - Create `internal/payment/reconciliation.go`
  - Read bank CSV exports (30 min/day task)
  - Match transfers to pending cards by reference ID
  - Update card status: pending → funded
  - Send funding message to Redis queue
  - **Estimated effort:** 4-6 hours implementation

- [ ] **Create admin dashboard for reconciliation**
  - Upload CSV endpoint
  - Match/unmatch interface
  - Manual override for edge cases
  - Reconciliation history log
  - **Estimated effort:** 8-10 hours

- [ ] **Update CreateCard API to include payment instructions**
  - Add bank account details to response
  - Generate unique reference ID for each card
  - Add payment deadline (24 hours)
  - Email payment instructions to customer
  - **Estimated effort:** 2-3 hours

### 1.2 Treasury Management - Automated OTC Purchases (Crypto.com)

**Priority:** HIGH  
**Cost Impact:** 0.16% (OTC) vs 3% (fiat onramp)  
**Automation Level:** Fully automatable via Crypto.com Exchange API

- [ ] **Set up Crypto.com Exchange account**
  - Register for Crypto.com Exchange (not the app)
  - Complete business KYC/AML verification
  - Enable API access with HMAC-SHA256 authentication
  - Whitelist treasury wallet address for withdrawals
  - Set up UAT sandbox for testing: `https://uat-api.3ona.co/exchange/v1/`
  - **Estimated effort:** 2-3 hours + KYC waiting time

- [ ] **Create treasury wallet system**
  - Database table: `treasury_wallets`
    - Fields: wallet_type, address, balance_sats, balance_fiat_cents, last_updated
  - Generate on-chain BTC address for receiving from OTC
  - Encrypt seed/private key with AES-256-GCM
  - Store encrypted key in secure location (consider HSM for production)
  - **Estimated effort:** 6-8 hours

- [ ] **Implement balance tracking**
  - Function: `GetTreasuryBalance()` - query blockchain + pending reservations
  - Function: `ReserveBalance(cardID, amountSats)` - Redis distributed lock
  - Function: `ReleaseReservation(cardID)` - unlock on success/failure
  - Database table: `balance_reservations`
  - **Estimated effort:** 4-6 hours

- [ ] **Implement automated OTC purchase flow (Crypto.com OTC 2.0 API)**
  - Create `internal/treasury/otc_provider.go`
  - **Step 1 - Fiat deposit to Crypto.com (via Bank API):**
    - Use bank API (Qonto/Revolut/Wise) to send SEPA wire to Crypto.com fiat wallet
    - Crypto.com provides SEPA deposit details via Fiat Wallet API (`openpayd_exchange_sepa`)
    - ⚠️ Fiat deposits CANNOT be initiated via Crypto.com API (must come from bank side)
    - Monitor deposit arrival via `private/user-balance` polling
  - **Step 2 - RFQ (Request for Quote):**
    - `POST private/otc/request-quote` with `{symbol: "BTCEUR", side: "BUY", size: amount}`
    - Response: quote with price, expiry (typically 10 seconds)
  - **Step 3 - Accept deal:**
    - `POST private/otc/request-deal` with `{quote_id: "..."}`
    - BTC credited instantly to exchange wallet
  - **Step 4 - Withdraw BTC to treasury:**
    - `POST private/create-withdrawal` with whitelisted treasury address
    - Monitor withdrawal status
  - **Full cycle time:** ~1-2 business days (SEPA) + instant (OTC buy + withdrawal)
  - **Estimated effort:** 10-12 hours

- [ ] **Implement treasury auto-refill trigger**
  - Monitor treasury balance via `GetTreasuryBalance()`
  - Trigger conditions:
    - Balance < 20% of weekly volume → Normal refill
    - Balance < 10% → Critical refill (immediate)
  - Auto-refill flow:
    1. Calculate refill amount (target: 1 week of expected volume)
    2. Send SEPA wire from bank to Crypto.com (via bank API)
    3. Wait for deposit confirmation (poll Crypto.com balance)
    4. Execute OTC buy (RFQ → Deal)
    5. Withdraw BTC to treasury wallet
  - Slack/email notifications at each step
  - **Estimated effort:** 6-8 hours

- [ ] **Set up treasury monitoring alerts**
  - Email/Slack alert at 20% balance
  - Critical alert at 10% balance
  - Daily balance summary email
  - Webhook integration with Slack
  - Monitor Crypto.com exchange balance separately
  - **Estimated effort:** 3-4 hours

### 1.3 Worker Implementation - Custodial Funding

**Priority:** HIGH  
**Status:** Skeleton exists, TODOs updated for custodial model

- [ ] **Implement `cmd/worker/fund_card/main.go`**
  - Consume FundCardMessage from Redis "fund_card" stream
  - Fetch BTC price from OTC provider (Crypto.com OTC 2.0 — our actual cost basis)
  - Calculate BTC amount (satoshis) for card's fiat value
  - Check treasury available balance (prevent overselling)
    - Available = Total Treasury (LN channels + hot wallet) - SUM(unredeemed card balances)
    - Use distributed lock (Redis SETNX) for concurrent worker safety
  - Update card: `btc_amount_sats = X`, `status = Active`, `funded_at = now`
  - Create Fund transaction record (accounting only — no tx_hash, no addresses)
  - **No blockchain transaction** — BTC stays in treasury, card is a balance claim
  - Handle errors: retry on transient (OTC API timeout, DB down), skip on permanent (card already funded)
  - **Estimated effort:** 6-8 hours
  - **Prerequisite:** `CardRepository.Fund(ctx, cardID, satoshis)` method with atomic update
    - SQL: `UPDATE cards SET btc_amount_sats = $2, status = 'active', funded_at = now() WHERE id = $1 AND status = 'funding'`

- [ ] **Add OTC price source to exchange provider**
  - Add `cryptocom_otc` provider to `internal/exchange/provider.go`
  - Use Crypto.com OTC 2.0 RFQ endpoint for indicative quotes
  - Fallback chain: OTC provider → Coinbase → CoinGecko
  - Cache price for 30 seconds (avoid hitting rate limits)
  - **Estimated effort:** 3-4 hours

### 1.4 Testing & Quality Assurance

- [ ] **Integration tests for full card lifecycle**
  - Test: Payment received → Card funded → Transaction confirmed
  - Test: Insufficient treasury balance handling
  - Test: Concurrent card funding (no double-spend)
  - Test: Transaction timeout and retry
  - **Estimated effort:** 8-10 hours

- [ ] **Load testing**
  - Simulate 100 cards/hour
  - Monitor Redis queue performance
  - Check database query performance
  - Identify bottlenecks
  - **Estimated effort:** 4-6 hours

- [ ] **Security audit**
  - Private key storage review
  - API authentication verification
  - SQL injection testing
  - Rate limiting validation
  - **Estimated effort:** 6-8 hours

---

## Phase 2: Automation & Optimization - Month 2

**Goal:** Automate manual processes and reduce operational overhead

### 2.1 Automated Bank Transfer Reconciliation

**Priority:** MEDIUM  
**Cost Impact:** €0-9/month (API costs) vs 30 min/day manual work

- [ ] **Integrate bank API for real-time payment notifications**
  - Create `internal/payment/bank_provider.go` (interface for multiple banks)
  - **If Qonto (recommended):**
    - OAuth 2.0 authentication
    - Trust Crypto.com as beneficiary → enables fully automated SEPA transfers (no SCA)
    - `POST /v2/external_transfers` for automated payouts to trusted beneficiaries
    - `POST /v2/sepa/bulk_transfers` for batch processing (up to 400 per batch)
    - Idempotency via `X-Qonto-Idempotency-Key` header
    - Instant SEPA by default (fallback to standard above threshold)
    - ⚠️ Transfers >€30,000 require at least one attachment
    - Sandbox available via Developer Portal (`X-Qonto-Staging-Token`)
  - **If Revolut Business:**
    - Bearer token auth (JWT), OAuth2, token expires 40 min
    - `POST /pay` endpoint (Company plans only, not Freelancer)
    - Counterparty management: Create, validate account name (CoP/VoP)
    - Webhooks v2: `TransactionCreated`, `TransactionStateChanged` events
    - Webhook retry: 3 times with 10-min intervals
    - ⚠️ Freelancer accounts must use `/payment-drafts` (manual approval)
    - Sandbox + Postman collection available
  - **If Wise Business:**
    - OAuth 2.0, client credentials + user tokens
    - Quote → Recipient → Transfer → Fund flow (4-step process)
    - `POST /v1/transfers` + `POST /v3/profiles/{id}/transfers/{id}/payments`
    - Batch groups: up to 1000 transfers in single funding (`POST /v3/profiles/{id}/batch-groups`)
    - Webhooks: `transfers#state-change`, `balances#credit` events
    - SCA protected for UK/EEA profiles (bypass with mTLS + client credentials)
    - Sandbox: `https://api.wise-sandbox.com/`
  - **Estimated effort:** 8-10 hours

- [ ] **Automated matching system**
  - Parse webhook/callback payload for reference ID
  - Auto-match transfers to cards in database
  - Auto-trigger funding workflow on match
  - Slack notification for unmatched transfers
  - Daily reconciliation report
  - **Estimated effort:** 4-6 hours

- [ ] **Handle edge cases**
  - Partial payments (wait for completion)
  - Overpayments (refund process)
  - Duplicate payments (idempotency)
  - Expired cards (auto-refund after 24h)
  - **Estimated effort:** 6-8 hours

### 2.2 Treasury Analytics Dashboard

**Priority:** MEDIUM

- [ ] **Build internal admin dashboard**
  - Real-time treasury balance display
  - Daily/weekly/monthly volume charts
  - Card funding success rate
  - Average confirmation time
  - Fee spending trends
  - **Estimated effort:** 12-16 hours

- [ ] **Automated reporting**
  - Weekly P&L summary
  - Cost breakdown (fees, OTC, operations)
  - Revenue by currency (EUR, USD, GBP)
  - Customer acquisition metrics
  - **Estimated effort:** 6-8 hours

### 2.3 Customer Experience Improvements

- [ ] **Email notifications**
  - Payment received confirmation
  - Card funding in progress
  - Card ready for redemption
  - Transaction confirmation updates
  - **Estimated effort:** 4-6 hours

- [ ] **Card redemption API**
  - Endpoint: `POST /api/cards/{id}/redeem`
  - Accept user's BTC address
  - Transfer funds from card wallet to user wallet
  - Broadcast transaction
  - Return tx_hash
  - **Estimated effort:** 6-8 hours

- [ ] **Card status tracking**
  - Public endpoint for checking card status
  - No authentication required (use card ID + security code)
  - Show: payment status, funding status, confirmations
  - **Estimated effort:** 3-4 hours

---

## Phase 3: Lightning Network Migration - Month 3-4

**Goal:** Reduce transaction costs by 99% and improve speed to <1 second

**Cost Impact:** €500 (on-chain) → €1 (Lightning) per 1000 cards

### 3.1 Lightning Infrastructure Setup

**Priority:** HIGH (if pursuing Lightning)  
**Prerequisites:** Phase 1 complete and generating revenue

- [ ] **Deploy LND node**
  - Server requirements: 4GB RAM, 100GB SSD, stable internet
  - Install LND (Lightning Network Daemon)
  - Sync Bitcoin node or use Neutrino (light client)
  - Configure LND for mainnet
  - Secure macaroon authentication
  - **Estimated effort:** 8-10 hours + 1-2 days sync time

- [ ] **Open Lightning channels**
  - Research hub selection (ACINQ, Bitrefill, LNBig)
  - Open channels with high-liquidity hubs
  - Channel size: 0.05-0.1 BTC per channel
  - Total channels: 3-5 for redundancy
  - **Cost:** €20-30 in channel opening fees (one-time)
  - **Estimated effort:** 4-6 hours

- [ ] **Set up channel monitoring**
  - Monitor channel balance (local vs remote)
  - Alert on low outbound liquidity
  - Automated channel rebalancing (loop out if needed)
  - Channel force-close detection
  - **Estimated effort:** 6-8 hours

### 3.2 Lightning Wallet Integration

- [ ] **Replace btcsuite with LND client**
  - Install `github.com/lightningnetwork/lnd/lnrpc`
  - Implement gRPC connection to LND
  - Create `internal/lightning/client.go`
  - Functions: CreateInvoice(), SendPayment(), DecodeInvoice()
  - **Estimated effort:** 8-10 hours

- [ ] **Update database schema for custodial model**
  ```sql
  -- Cards are balance claims on treasury. No wallets, no keys, just amounts.
  -- btc_amount_sats tracks remaining balance (decremented on each spend)
  -- Status: created → funding → active → redeemed (when balance = 0)
  -- No redemption_method on cards — each transaction tracks its own method
  
  -- ALREADY DONE: Removed wallet_address, encrypted_priv_key from cards
  -- ALREADY DONE: Added redemption_method, payment_hash, payment_preimage,
  --               lightning_invoice to transactions table
  ```
  - ~~Migration script to remove wallet fields~~ ✅ Done
  - Much simpler and more secure than managing 1000s of private keys
  - **Partial spend model:** Cards can be spent in portions (multiple transactions)
    - Each transaction deducts from `btc_amount_sats`
    - Card stays `active` until balance reaches 0, then becomes `redeemed`
    - Each transaction independently chooses Lightning or on-chain
  - **Estimated effort:** 2-3 hours

- [ ] **Update CreateCard for custodial model**
  - Store card value in database (balance in sats)
  - Card represents a CLAIM on our treasury
  - No Bitcoin transaction happens yet (just accounting)
  - **Card funding flow:** 
    - User pays €100 via bank transfer
    - Create card with balance: 0.0019 BTC (€95 after 5% fee)
    - BTC stays in our Lightning channels - card is just a database entry
    - User receives card details (card ID + redemption code)
  - **Note:** BTC is only transferred when user redeems, not when card is created
  - **Estimated effort:** 4-6 hours

### 3.3 Custodial Treasury System

**Architecture:** OTC (on-chain) → Treasury On-Chain Wallet → Lightning Channels (BTC locked) → Users redeem on-demand

**How it works:**
1. **Receive from OTC:** BTC arrives at treasury on-chain address (example: 2 BTC received)
2. **Split Treasury:**
   - **Lightning Channels:** Lock 1.8 BTC (90%) - for Lightning redemptions
   - **Hot Wallet:** Keep 0.2 BTC (10%) on-chain - for on-chain redemptions
3. **Create Cards:** Database entries with balance claims (NO Bitcoin tx, NO individual wallets)
4. **User Redeems (Lightning):** Pay from Lightning channel balance → User's Lightning wallet
5. **User Redeems (On-Chain):** Send from hot wallet → User's on-chain address

**Important:** 
- Cards are custodial - NO individual wallets created per card
- We hold ALL BTC in OUR treasury (Lightning channels + hot wallet)
- Card creation is just accounting - BTC only moves when user redeems
- Lightning channels can ONLY send Lightning payments (that's why we need hot wallet for on-chain)

- [ ] **Implement treasury management system**
  - On-chain wallet: Receive from OTC providers (primary treasury)
  - Lightning channels: Lock 80-90% of treasury for instant Lightning redemptions
  - Hot wallet: Keep 10-20% on-chain for users who need on-chain (exchanges, hardware wallets)
  - **Database:** Track total treasury balance vs reserved balances (unredeemed cards)
  - **Formula:** Available = Total Treasury - Sum(Unredeemed Card Balances)
  - **Why both?** Lightning adoption is growing but not universal. Maximize market reach.
  - **Estimated effort:** 6-8 hours

- [ ] **Automated channel liquidity management**
  - Monitor outbound capacity daily
  - Refill channels from on-chain treasury
  - Use submarine swaps (Lightning Loop) if needed
  - Alert when channels below 20% capacity
  - **Estimated effort:** 8-10 hours

- [ ] **Channel opening automation**
  - Function: `OpenChannel(nodeID, amountSats)`
  - Trigger: When outbound liquidity < 10%
  - Source: On-chain treasury wallet
  - Confirmation: Wait for 3 confirmations before using
  - **Estimated effort:** 4-6 hours

### 3.4 Worker Update - Lightning-First Funding

- [ ] **Update `cmd/worker/fund_card/main.go` for hybrid mode**
  - Check card type: Lightning invoice or on-chain address
  - **Lightning path:**
    - Decode invoice
    - Check invoice expiry
    - Send payment via LND `SendPaymentSync()`
    - Update card status on success (instant)
    - Cost: €0.001, Time: 1 second
  - **On-chain fallback:**
    - Use existing on-chain logic
    - Cost: €0.50, Time: 30-60 minutes
  - **Estimated effort:** 6-8 hours

- [ ] **Add Lightning payment monitoring**
  - Subscribe to LND payment updates via gRPC stream
  - Handle payment failures (routing, insufficient liquidity)
  - Retry logic: Try 3 times, then fallback to on-chain
  - Log payment routes for optimization
  - **Estimated effort:** 6-8 hours

### 3.5 Testing Lightning Integration

- [ ] **Testnet validation**
  - Deploy LND on Bitcoin testnet
  - Open channels with testnet faucets
  - Test invoice generation and payment
  - Test failure scenarios (expired invoice, routing failure)
  - **Estimated effort:** 8-10 hours

- [ ] **Mainnet pilot**
  - Start with 10 Lightning cards
  - Monitor success rate
  - Measure actual costs and speed
  - Gather user feedback
  - **Estimated effort:** 4-6 hours + monitoring time

---

## Phase 4: Lightning-First Redemption - Month 5

**Goal:** Default to Lightning redemption with on-chain fallback for maximum adoption

**Strategy:** 90% Lightning (instant, €0.001) + 10% on-chain (30 min, €0.50)
**User Compatibility Analysis (2026):**
- Lightning wallets: Phoenix, Muun, Wallet of Satoshi, BlueWallet (~40% of users)
- Exchange wallets: Coinbase, Binance, Kraken (support Lightning withdrawals only)
- Hardware wallets: Ledger, Trezor (on-chain only) (~20% of users)
- **Reality:** Most users CAN receive Lightning, but many prefer familiar on-chain

### 4.1 Database Schema Updates

- [x] **Move redemption fields to transactions table** ✅ Done
  ```sql
  -- Transactions table now tracks per-spend details:
  -- redemption_method TEXT NULL     — 'lightning' or 'onchain' (per transaction)
  -- payment_hash VARCHAR(64) NULL   — Lightning payment identifier
  -- payment_preimage VARCHAR(64) NULL — Lightning proof of payment
  -- lightning_invoice TEXT NULL      — BOLT11 invoice string
  -- tx_hash VARCHAR(64) NULL        — On-chain tx hash (existing)
  ```
  - Each spend creates a new transaction with its own method
  - Cards support partial spends (multiple redeems until balance = 0)
  - `btc_amount_sats` on Card = remaining balance (decremented per spend)

### 4.2 Redemption API Updates

- [ ] **Update `POST /api/cards/{id}/redeem` endpoint**
  - Accept request body:
    ```json
    {
      "method": "lightning",
      "lightning_invoice": "lnbc...",
      "amount_sats": 50000
    }
    ```
    OR
    ```json
    {
      "method": "onchain",
      "address": "bc1q...",
      "amount_sats": 100000
    }
    ```
  - **Partial spend support:** `amount_sats` can be less than card balance
  - Creates a Transaction record with `redemption_method`, Lightning or on-chain fields
  - Deducts `amount_sats` from card's `btc_amount_sats`
  - Card stays `active` until balance = 0, then becomes `redeemed` with `redeemed_at` set
  - **Lightning redemption:** Pay user's invoice from our channel (instant, FREE)
  - **On-chain redemption:** Send from hot wallet to user's address (30 min, fee deducted)
  - Validate Lightning invoice amount matches requested `amount_sats`
  - Validate on-chain address (checksum, network)
  - **Estimated effort:** 6-8 hours

- [ ] **Implement dual redemption worker**
  - New worker: `cmd/worke (90% of users with Lightning wallets):**
    - Pay user's Lightning invoice from our channel balance
    - Lightning → Lightning (both sides stay in Lightning Network)
    - Update card status immediately on payment success
    - Cost: €0.001, Time: 1 second
  - **On-chain redemption (10% of users with exchange wallets):**
    - Send from hot wallet (on-chain reserve) to user's address
    - Wait for confirmations
    - Cost: €0.50, Time: 30 minutes
    - **Note:** This is why we keep 10-20% of treasury on-chain (not in channels)address
    - Wait for confirmations
    - Cost: €0.50, Time: 30 minutes
  - **Estimated effort:** 8-10 hours

### 4.3 User Experience - Lightning First

- [ ] **Smart redemption UI with Lightning default**
  - **Default:** Lightning option selected (instant, free)
  - **Alternative:** "Use standard Bitcoin address instead" link (slower, €0.50 fee)
  - Show clear benefits: "⚡ Instant & FREE" vs "🐌 30 min wait + €0.50 fee"
  - QR code generation for Lightning invoices
  - Address validation with real-time feedback
  - **Psychology:** Make Lightning the path of least resistance
  - **Estimated effort:** 10-12 hours

- [ ] **Lightning onboarding flow**
  - Detect if user has Lightning wallet (paste invoice vs address)
  - Recommend Lightning wallets if on-chain chosen: Phoenix (easiest), Muun, Wallet of Satoshi
  - "Try Lightning - Get your BTC instantly!" banner
  - Educational tooltip: "Lightning = Same Bitcoin, instant delivery, no fees"
  - Track redemption method choices (measure Lightning adoption)
  - **Goal:** Push 90%+ to Lightning through UX, not force
  - **Estimated effort:** 6-8 hours

- [ ] **"Don't redeem, spend it!" teaser (pre-Phase 5)**
  - After redemption, show: "Coming soon: Spend your gift card directly at merchants ⚡"
  - Email capture for waitlist: "Be first to spend BTC at your favorite stores"
  - Track interest level (measure demand for payment features)
  - **Purpose:** Educate users that Lightning = payments, not just transfers
  - **Estimated effort:** 2-3 hours

---

## Phase 5: Merchant Payments - Month 6-8

**Goal:** Transform gift cards from a transfer tool into a **payment instrument**

> Instead of: Buy card → Redeem to wallet → Send to merchant  
> We enable: Buy card → **Pay merchant directly** from card balance

**Why this matters:**
- Users keep BTC in our ecosystem (longer retention, more revenue)
- Every payment = a fee opportunity (1-2% merchant fee)
- Merchants get instant settlement via Lightning (no 3-5 day bank wait)
- Positions us as a **payment platform**, not just a gift card shop

### 5.1 Direct Card Payments (Card-to-Merchant Lightning)

**Priority:** HIGH  
**Revenue Impact:** 1-2% per transaction (recurring vs one-time card fee)

- [ ] **Implement partial balance spending**
  - Update database: Cards can have partial redemptions
  - Track transaction history per card (not just one-time redeem)
  - New field: `remaining_balance_sats`
  - Card becomes a **prepaid Lightning wallet**
  - **Estimated effort:** 6-8 hours

- [ ] **Payment API for merchants**
  - Endpoint: `POST /api/cards/{id}/pay`
  - Request: `{ "amount_sats": 50000, "merchant_invoice": "lnbc..." }`
  - Validate card balance >= payment amount
  - Pay merchant's Lightning invoice from our channel
  - Deduct from card balance (partial spend)
  - Return payment confirmation + remaining balance
  - **Estimated effort:** 8-10 hours

- [ ] **Merchant onboarding portal**
  - Merchant registration with Lightning address/node
  - API keys for POS integration
  - Merchant dashboard: payments received, settlement history
  - Fee structure: 1-2% per transaction (competitive vs Visa's 2-3%)
  - **Estimated effort:** 16-20 hours

- [ ] **QR code payment flow**
  - Merchant displays QR code with Lightning invoice
  - User scans QR with our web app using card ID
  - One-tap payment from card balance
  - Instant confirmation for both parties
  - **Estimated effort:** 8-10 hours

### 5.2 Lightning Address Integration

- [ ] **Assign Lightning addresses to cards**
  - Each card gets: `card-{id}@ourgiftcard.com`
  - Cards can RECEIVE Lightning payments (top-up from friends)
  - Cards can SEND Lightning payments (pay merchants)
  - Makes cards feel like real wallets
  - **Estimated effort:** 10-12 hours

- [ ] **LNURL-pay support**
  - Implement LNURL-pay for seamless merchant payments
  - Static QR codes for merchants (no new invoice each time)
  - Support comments/memos on payments
  - **Estimated effort:** 6-8 hours

### 5.3 Multi-Currency Support

- [ ] **Add USD, GBP support**
  - Update database: Support multiple fiat currencies
  - Currency conversion via exchange APIs
  - Display prices in user's local currency
  - Show card balance in both BTC and fiat equivalent
  - **Estimated effort:** 12-16 hours

### 5.4 Marketing & Growth

- [ ] **Referral program**
  - Unique referral codes
  - 5% commission on referred sales
  - Referral dashboard
  - **Estimated effort:** 10-12 hours

- [ ] **Gift card customization**
  - Custom messages on cards
  - Branding options for B2B
  - Scheduled delivery (birthdays, holidays)
  - **Estimated effort:** 12-16 hours

- [ ] **"Pay with BTC Gift Card" badges for merchants**
  - Embeddable payment buttons for websites
  - "We accept BTC Gift Cards" stickers for physical stores
  - Co-marketing with early merchant partners
  - **Estimated effort:** 4-6 hours

---

## Phase 6: Payment Ecosystem - Year 2+

**Goal:** Full payment platform with virtual cards, NFC, and merchant network

> From gift card company → **Lightning payment network**

### 6.1 Virtual Debit Cards

- [ ] **Issue virtual Visa/Mastercard linked to card balance**
  - Partner with card issuer (e.g., Reap, Immersve, or similar crypto-card provider)
  - User links gift card balance to virtual card
  - Spend anywhere Visa/Mastercard is accepted
  - Auto-convert BTC → EUR at point of sale
  - **Estimated effort:** Research + partnership (2-3 months)

### 6.2 NFC Tap-to-Pay

- [ ] **Physical card with NFC chip**
  - Tap-to-pay at Lightning-enabled terminals
  - BoltCard standard (open-source NFC + Lightning)
  - Premium product: physical gift card with NFC
  - **Estimated effort:** Hardware partnership + 40-60 hours development

### 6.3 Merchant Network

- [ ] **Build merchant directory**
  - Map of merchants accepting our gift cards
  - Categories: restaurants, online stores, services
  - Merchant reviews and ratings
  - **Estimated effort:** 20-30 hours

- [ ] **Merchant SDK / plugins**
  - WooCommerce plugin for online stores
  - Shopify integration
  - POS integration (Square, SumUp)
  - **Estimated effort:** 40-60 hours

### 6.4 Bulk / B2B Solutions

- [ ] **B2B endpoint for bulk card creation**
  - Endpoint: `POST /api/cards/bulk`
  - Create 10-1000 cards in one request
  - Discount pricing tiers for businesses
  - CSV export of card details
  - Use cases: employee rewards, customer incentives, event giveaways
  - **Estimated effort:** 8-10 hours

- [ ] **Corporate gift card program**
  - White-label gift cards for businesses
  - Custom branding and messaging
  - Analytics dashboard for corporate clients
  - **Estimated effort:** 20-30 hours

### 6.5 Advanced Security

- [ ] **Multi-signature treasury**
  - Require 2-of-3 signatures for large withdrawals
  - Hardware wallet integration (Ledger, Trezor)
  - Separate hot/cold wallet system
  - **Estimated effort:** 16-20 hours

- [ ] **Rate limiting and DDoS protection**
  - Implement Redis-based rate limiting
  - Cloudflare integration
  - API key system for partners
  - **Estimated effort:** 6-8 hours

---

## Cost-Benefit Analysis

### Current MVP (On-Chain Only)
**Per 1000 cards:**
- Revenue: €5,000 (5% fee)
- Costs: €2,485 (Stripe 0.25% + on-chain €0.50)
- Profit: €2,515 (50.3% margin)

### Phase 2 Optimization (Direct Bank + On-Chain)
**Per 1000 cards:**
- Revenue: €5,000
- Costs: €841 (€0 bank + €0.50 on-chain + €341 OTC)
- Profit: €4,159 (83.2% margin)
- **Improvement:** +€1,644 profit (+65%)

### Phase 3 Migration (Direct Bank + Lightning)
**Per 1000 cards:**
- Revenue: €5,000
- Costs: €637 (€0 bank + €0.001 Lightning + €341 OTC + €20 channels)
- Profit: €4,363 (87.3% margin)
- **Improvement:** +€1,848 profit (+73% vs MVP)

---

## Risk Mitigation

### Technical Risks

- **Lightning Network Complexity**
  - Mitigation: Start on testnet, pilot with 10 cards, gradual rollout
  - Fallback: Keep on-chain system as backup

- **Channel Liquidity Issues**
  - Mitigation: Monitor daily, set up automated alerts, maintain hot wallet
  - Fallback: On-chain funding if Lightning fails

- **LND Node Downtime**
  - Mitigation: Hot standby node, automated failover, health checks
  - Fallback: Queue cards until node recovers

### Business Risks

- **Low Volume (Treasury Overinvestment)**
  - Mitigation: Start with €5K treasury, scale based on demand
  - Trigger: Refill only when processing >50 cards/week

- **OTC Provider Delays**
  - Mitigation: 2-3 business day buffer in treasury
  - Minimum treasury: 1 week of expected volume

- **Regulatory Compliance**
  - Mitigation: Consult with crypto-friendly legal advisor
  - KYC/AML: Implement if volume exceeds regulatory thresholds

---

## Decision Points

### Critical Decisions Needed

1. **Lightning Migration Timeline**
   - ⏳ Decision: Proceed with Phase 3 (Month 3-4) or stay on-chain indefinitely?
   - **Recommendation:** Migrate after 500 successful on-chain cards
   - **Criteria:** Revenue > €2,500, operational stability, team capacity

2. **Bank Transfer Provider**
   - ⏳ Decision: Qonto vs Revolut Business vs Wise Business vs bunq?
   - **Recommendation:** Qonto (French-regulated, API on all plans, fully automated SEPA to trusted beneficiaries, instant SEPA by default)
   - See **Appendix A** below for full comparison
   - **Criteria:** Webhook/notification support, API quality, SCA automation, monthly fees, SEPA instant support

3. **Redemption Strategy**
   - ⏳ Decision: Lightning-only or Lightning-first with on-chain fallback?
   - **Recommendation:** Lightning-first hybrid (reach 100% of users, push 85-90% to Lightning through UX)
   - **Why not Lightning-only?** Would exclude 20-40% of potential customers (exchange-only users, hardware wallets)
   - **Why not equal treatment?** Lightning is objectively better (instant, free) - make it the default
   - **Criteria:** Adoption metrics (track % choosing Lightning), customer feedback

4. **OTC Provider Selection**
   - ⏳ Decision: Crypto.com OTC vs Kraken OTC vs Binance OTC?
   - **Recommendation:** Crypto.com OTC (fully automatable API, RFQ flow, sandbox available)
   - **Comparison:**

     | Feature | Crypto.com OTC 2.0 | Kraken OTC | Binance OTC |
     |---------|-------------------|------------|-------------|
     | API automation | ✅ Full RFQ → Deal flow | ❌ Contact desk | ⚠️ Limited |
     | BTC withdrawal API | ✅ `create-withdrawal` | ✅ API available | ✅ API available |
     | Fiat deposit API | ❌ Must wire from bank | ❌ Must wire from bank | ❌ Must wire from bank |
     | Fiat deposit methods | SEPA, SWIFT, Fedwire, UK FPS | SEPA, SWIFT | SEPA, SWIFT |
     | Sandbox/UAT | ✅ `uat-api.3ona.co` | ❌ No sandbox | ❌ No sandbox |
     | Auth method | HMAC-SHA256 | API key + nonce | HMAC-SHA256 |
     | Quote validity | ~10 seconds | Manual negotiation | Varies |
     | EU compliance | ✅ Licensed | ✅ Licensed | ⚠️ Regulatory pressure |

   - **Criteria:** Full API automation, sandbox for testing, EU regulatory status

---

## Success Metrics

### Phase 1 (MVP Launch)
- ✅ 100 cards created successfully
- ✅ 95% payment success rate
- ✅ Average funding time: <90 minutes
- ✅ Zero treasury overdraft incidents
- ✅ <1% customer support tickets

### Phase 2 (Automation)
- ✅ 90% automated bank Infrastructure)
- ✅ Lightning channels operational with 90% treasury capacity
- ✅ Channel uptime: >99.5%
- ✅ Transaction cost: <€0.01 per redemption
- ✅ Zero failed payments (automatic on-chain fallback)

### Phase 4 (Lightning-First Redemption)
- ✅ **Target:** 85-90% users choose Lightning (through smart UX + free redemption)
- ✅ 10-15% users use on-chain fallback (exchange wallets, hardware wallets)
- ✅ 100% redemption success rate (hybrid system ensures no failures)
- ✅ Average redemption time: <5 seconds (Lightning) or <30 minutes (on-chain)
- ✅ Customer satisfaction: "Instant delivery" vs competitors' 30+ minute wait
- ✅ 20%+ users sign up for "spend at merchants" waitlist

### Phase 5 (Merchant Payments)
- ✅ 10+ merchants onboarded
- ✅ 30% of cards used for payments (not just redemption)
- ✅ Average card lifetime: >2 transactions (partial spending)
- ✅ Merchant settlement time: <5 seconds
- ✅ Payment success rate: >99%

### Phase 6 (Payment Ecosystem)
- ✅ 100+ merchants in network
- ✅ Virtual card integration active
- ✅ 50% of revenue from payment fees (vs card creation fees)
- ✅ Gift card → Payment wallet conversion: 40%+ of users

---

## Resources & Budget

### Infrastructure Costs

**Phase 1 (On-Chain MVP):**
- Server: €10-20/month (DigitalOcean/Hetzner)
- Database: Included in server
- Redis: Included in server
- Total: €10-20/month

**Phase 3 (Lightning):**
- LND Server: €20-40/month (dedicated server)
- Channel opening fees: €20-30 (one-time)
- Channel rebalancing: €5-10/month
- Total: €25-50/month

### Development Time Estimates

- **Phase 1:** 60-80 hours (1.5-2 months part-time)
- **Phase 2:** 40-60 hours (1 month part-time)
- **Phase 3:** 60-80 hours (1.5-2 months part-time)
- **Phase 4:** 40-60 hours (1 month part-time)
- **Phase 5:** 80-100 hours (2-3 months part-time) ← Merchant payments
- **Phase 6:** 120-160 hours (ongoing) ← Payment ecosystem
- **Total (MVP → Payments):** 280-380 hours (7-10 months part-time)

### Treasury Investment

- **Initial:** €5,000 (bootstrap phase)
- **Month 2:** €10,000 (first automated OTC purchase via Crypto.com)
- **Month 3:** €20,000 (scale to 200 cards/week)
- **Month 6:** €50,000+ (institutional volume)

### Revenue Evolution

- **Phase 1-4:** Revenue from 5% card creation fee only
- **Phase 5:** + 1-2% merchant payment fee (recurring revenue per card)
- **Phase 6:** + Virtual card fees, NFC card sales, B2B partnerships
- **Long-term:** Payment fees > Card creation fees (sustainable recurring revenue)

---

---

## Architecture Summary: Custodial Model

### How It Works

**Cards are custodial balance claims, NOT individual wallets:**
fast, cheap redemptions - DEFAULT path)
   - Hot wallet: 10-20% on-chain (for users who need on-chain - FALLBACK path)
2. **Cards are database entries** - Each card has a `balance_sats` field representing their claim
3. **No wallet per card** - We don't generate addresses or private keys for cards
4. **BTC transfers only on redemption:**
   - **Lightning redemption (90% of users):** Pay from Lightning channel balance (instant, €0.001)
   - **On-chain redemption (10% of users):** Send from hot wallet (30 min, €0.50)
   
**Market Reality (2026):**
- Lightning adoption growing but not universal (~40-60% have Lightning wallet capability)
- Exchanges support Lightning withdrawals but users still often deposit to exchange wallets
- Hardware wallet users (security-conscious) prefer on-chain
- **Solution:** Make Lightning the easy path, keep on-chain available
   - Lightning redemption → Pay from Lightning channel balance
   - On-chain redemption → Send from hot wallet

**Card Creation = Accounting, NOT Transaction:**
- User pays €100 → Card created with balance 0.0019 BTC
- No Bitcoin movement yet
- BTC stays in our Lightning channels/hot wallet
- Card is a promise to pay that amount later

**Card Redemption = Actual Bitcoin Transfer:**
- User provides Lightning invoice → We pay from our Lightning channel (instant, €0.001)
- User provides on-chain address → We send from hot wallet (30 min, €0.50)
- Card balance set to 0, marked as redeemed

**Treasury Balance Formula:**
```
Total Treasury = On-Chain + Lightning Channels
Available Balance = Total - Sum(Unredeemed Card Balances)
```

Example:
- Total Treasury: 2 BTC
- Unredeemed cards: 100 cards × 0.0019 BTC = 0.19 BTC
- Available: 2 - 0.19 = 1.81 BTC (can create more cards)

---

## Appendix A: Bank API Comparison

### Full Feature Comparison

| Feature | Qonto | Revolut Business | Wise Business | bunq |
|---------|-------|-----------------|---------------|------|
| **Regulation** | French ACPR | Lithuanian EMI (EU passport) | Multi-jurisdiction EMI | Dutch banking license (DNB) |
| **API availability** | All plans (incl. Basic) | Company plans only for `/pay` | All plans | All plans |
| **Auth method** | OAuth 2.0 + Bearer | Bearer JWT + OAuth2 (40min expiry) | OAuth 2.0 + Client Credentials | API key + request signing |
| **Outgoing SEPA** | ✅ `POST /v2/external_transfers` | ✅ `POST /pay` (Company only) | ✅ Quote→Recipient→Transfer→Fund (4 steps) | ✅ `POST /payment` |
| **Batch payments** | ✅ Up to 400 per batch | ❌ One at a time | ✅ Up to 1000 (Batch Groups) | ✅ Up to 350 (XML) |
| **SEPA instant** | ✅ Default (fallback to standard) | ✅ Supported | ⚠️ Depends on route | ✅ Supported |
| **Webhooks** | ⚠️ Limited (polling recommended) | ✅ v2: `TransactionCreated`, `TransactionStateChanged` | ✅ `transfers#state-change`, `balances#credit` | ✅ `MUTATION`, `PAYMENT` categories |
| **SCA bypass** | ✅ Trusted beneficiaries = no SCA | ⚠️ Company plan only, mTLS optional | ✅ mTLS + client credentials = no SCA | ⚠️ App-based confirmation |
| **Idempotency** | ✅ `X-Qonto-Idempotency-Key` | ✅ `request_id` field | ✅ `customerTransactionId` | ✅ `X-Bunq-Client-Request-Id` |
| **Sandbox** | ✅ Developer Portal | ✅ Available + Postman | ✅ `api.wise-sandbox.com` | ✅ `sandbox.bunq.com` |
| **SDKs** | ❌ REST only | ❌ REST only | ❌ REST only | ✅ Python, Java, C#, PHP |
| **Account balances** | ✅ API | ✅ `GET /accounts` | ✅ `GET /v4/profiles/{id}/balances` | ✅ API |
| **Multi-currency** | ⚠️ EUR-focused | ✅ 30+ currencies | ✅ 50+ currencies, best FX rates | ✅ EUR-focused |
| **Monthly fee** | From €9/month (Basic) | From €0 (Free), €25 (Grow) | From €0 (pay-per-use) | From €8.99/month |
| **Key limitation** | >€30K transfers need attachment | Freelancer plan = payment drafts only | 4-step transfer flow (complex) | Complex auth (request signing) |

### Automation Path for Treasury Refill

**Qonto (Recommended):**
1. One-time: Trust Crypto.com's SEPA beneficiary details → No SCA required for future transfers
2. `POST /v2/external_transfers` with Crypto.com IBAN + amount + idempotency key
3. Instant SEPA (arrives in seconds-minutes) → Crypto.com detects deposit
4. **Result:** Fully automated fiat-to-exchange pipeline, no human intervention

**Revolut Business:**
1. Create counterparty: `POST /counterparty` with Crypto.com bank details
2. `POST /pay` with `account_id`, `receiver.counterparty_id`, `amount`, `currency`, `reference`
3. Webhook notification on `TransactionStateChanged` for confirmation
4. **Result:** Fully automated, but requires Company plan (€25+/month)

**Wise Business:**
1. Create recipient: `POST /v1/accounts` with Crypto.com bank details
2. Create quote: `POST /v3/profiles/{id}/quotes` (sourceAmount, EUR→EUR)
3. Create transfer: `POST /v1/transfers` with quote + recipient
4. Fund transfer: `POST /v3/profiles/{id}/transfers/{id}/payments` (type: BALANCE)
5. Track: Subscribe to `transfers#state-change` webhook
6. **Result:** Fully automated but 4-step flow, best for multi-currency (EUR→USD, GBP→EUR)

**bunq:**
1. Create payment: `POST /v1/user/{id}/monetary-account/{id}/payment`
2. Webhook via `notification-filter-url` with `PAYMENT` category
3. **Result:** Fully automated, simple API, but Dutch banking license (strong regulation)

### Recommendation

**Primary: Qonto** — Best for our use case because:
- ✅ Trusted beneficiary = fully automated transfers without SCA (critical for automation)
- ✅ SEPA instant by default (fastest fiat delivery to Crypto.com)
- ✅ API on all plans (no premium plan required for API access)
- ✅ French-regulated (ACPR) — strong EU compliance
- ✅ Batch transfers up to 400 (useful for refunds)
- ✅ Idempotency support (safe retries)
- ⚠️ EUR-focused (fine for EU-based business)

**Secondary: Revolut Business** (if multi-currency needed or already using Revolut):
- ✅ 30+ currencies, good webhook support
- ⚠️ Requires Company plan for API payments (€25+/month)

**Tertiary: Wise Business** (if sending to non-SEPA destinations):
- ✅ Best FX rates, 50+ currencies
- ✅ Ideal if treasury refill involves USD/GBP conversion
- ⚠️ More complex 4-step transfer flow

---

## Appendix B: Automated Treasury Refill Flow

### End-to-End Automation: Bank → Crypto.com → Treasury

```
┌─────────────────────────────────────────────────────────────────────┐
│                    AUTOMATED TREASURY REFILL                        │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│  1. TRIGGER: Treasury balance < 20% of weekly volume                │
│     └─ internal/treasury/monitor.go polls every 5 minutes           │
│                                                                     │
│  2. BANK TRANSFER (Qonto API):                                     │
│     └─ POST /v2/external_transfers                                  │
│        ├─ To: Crypto.com SEPA details (trusted beneficiary)         │
│        ├─ Amount: 1 week of expected volume (e.g., €10,000)         │
│        ├─ Reference: "TREASURY-REFILL-{timestamp}"                  │
│        └─ SEPA Instant → arrives in seconds                         │
│                                                                     │
│  3. WAIT FOR DEPOSIT (Crypto.com API):                              │
│     └─ Poll: POST private/user-balance every 60 seconds             │
│        └─ Check EUR balance increase                                │
│                                                                     │
│  4. OTC BUY (Crypto.com OTC 2.0 API):                              │
│     ├─ POST private/otc/request-quote {BTCEUR, BUY, amount}        │
│     ├─ Receive quote (valid ~10 seconds)                            │
│     └─ POST private/otc/request-deal {quote_id}                    │
│        └─ BTC credited instantly to exchange wallet                 │
│                                                                     │
│  5. WITHDRAW BTC (Crypto.com Wallet API):                           │
│     └─ POST private/create-withdrawal                               │
│        ├─ To: Whitelisted treasury wallet address                   │
│        ├─ Amount: BTC purchased                                     │
│        └─ Monitor: Poll withdrawal status                           │
│                                                                     │
│  6. CONFIRM: BTC arrives at treasury wallet                         │
│     └─ Update treasury balance in database                          │
│     └─ Slack notification: "Treasury refilled: +X BTC"              │
│                                                                     │
│  Total time: ~5 min (SEPA instant) to ~1 day (standard SEPA)       │
│  Total fees: ~0.16% OTC + SEPA transfer fee (~€0-1)                │
│  Human intervention: NONE                                           │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

### API Authentication Summary

| Service | Auth Method | Token Lifetime | Sandbox |
|---------|------------|----------------|---------|
| Qonto | OAuth 2.0 Bearer | Session-based | `X-Qonto-Staging-Token` |
| Revolut | JWT Bearer + OAuth2 | 40 minutes (refresh available) | Postman + sandbox env |
| Wise | OAuth 2.0 + Client Credentials | Session-based | `api.wise-sandbox.com` |
| bunq | API key + HMAC request signing | Per session | `sandbox.bunq.com` |
| Crypto.com | HMAC-SHA256 | Per request | `uat-api.3ona.co` |

---

## Notes

- This roadmap is subject to change based on user feedback and market conditions
- Prioritize user experience and security over speed of implementation
- Test thoroughly on testnet before any mainnet deployment
- Keep detailed logs of all treasury transactions for accounting
- Stay updated on regulatory requirements for crypto businesses in your jurisdiction
- **Strategic priority:** Every decision should move users toward Lightning Network adoption
- **North star metric:** % of cards used for payments (not just one-time redemption)
- Gift cards are the entry point — Lightning payments are the destination

---

**Next Actions:**
1. Review and approve this roadmap
2. Make decision on Lightning migration timeline
3. Choose bank transfer provider (Qonto recommended — see Appendix A)
4. Choose OTC provider (Crypto.com recommended — see Decision Point #4)
5. Set up business bank account + Crypto.com Exchange account
6. Test automation pipeline in sandboxes (Qonto staging + Crypto.com UAT)
7. Begin Phase 1 implementation
8. Research merchant payment regulations (payment license requirements)
9. Identify 5-10 pilot merchants for Phase 5 (crypto-friendly businesses)
