package wallet

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"btc-giftcard/pkg/logger"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"go.uber.org/zap"
)

type Wallet struct {
	PrivateKey string // WIF format
	PublicKey  []byte // Compressed public key (33 bytes)
	Address    string // bc1q... format
	Network    string // "mainnet" or "testnet"
}

type UTXO struct {
	TxHash string `json:"txid"`
	Vout   uint32 `json:"vout"`
	Value  int64  `json:"value"`
	Status struct {
		Confirmed   bool `json:"confirmed"`
		BlockHeight int  `json:"block_height"`
	} `json:"status"`
}

// getNetworkConfig returns network parameters for mainnet or testnet
func getNetworkConfig(network string) *chaincfg.Params {
	var params *chaincfg.Params
	if network == "mainnet" {
		params = &chaincfg.MainNetParams
	} else {
		params = &chaincfg.TestNet3Params
	}
	return params
}

// GenerateWallet creates a new random Bitcoin wallet with SegWit (bc1/tb1) address.
// Supported networks: "mainnet" or "testnet".
func GenerateWallet(network string) (*Wallet, error) {
	// 1. Get network parameters (mainnet or testnet)
	if network != "mainnet" && network != "testnet" {
		return nil, errors.New("invalid network: must be 'mainnet' or 'testnet'")
	}

	params := getNetworkConfig(network)

	// 2. Generate random private key
	privKey, err := btcec.NewPrivateKey()
	if err != nil {
		logger.Error("Failed to generate private key", zap.Error(err))
		return nil, err
	}

	// 3. Derive public key from private key
	publicKey := privKey.PubKey()

	// 4. Generate SegWit address (bc1q...) from public key
	// Create witness program hash (P2WPKH - Pay to Witness Public Key Hash)
	pubKeyHash := btcutil.Hash160(publicKey.SerializeCompressed())
	address, err := btcutil.NewAddressWitnessPubKeyHash(pubKeyHash, params)
	if err != nil {
		logger.Error("Failed to generate address", zap.Error(err))
		return nil, err
	}

	// 5. Convert private key to WIF format (compressed key)
	wif, err := btcutil.NewWIF(privKey, params, true)
	if err != nil {
		logger.Error("Failed to convert private key to WIF", zap.Error(err))
		return nil, err
	}

	// 6. Return Wallet struct
	return &Wallet{
		PrivateKey: wif.String(),
		PublicKey:  publicKey.SerializeCompressed(),
		Address:    address.EncodeAddress(),
		Network:    network,
	}, nil
}

// ValidateAddress checks if a Bitcoin address is valid for the specified network.
// Returns false if the address format is invalid or network doesn't match.
func ValidateAddress(address string, network string) (bool, error) {
	if network != "mainnet" && network != "testnet" {
		return false, errors.New("invalid network: must be 'mainnet' or 'testnet'")
	}

	params := getNetworkConfig(network)

	// Use btcutil.DecodeAddress
	btcAddress, err := btcutil.DecodeAddress(address, params)
	if err != nil {
		logger.Warn("Invalid Bitcoin address", zap.String("address", address), zap.Error(err))
		return false, nil
	}

	// Check network matches
	if !btcAddress.IsForNet(params) {
		logger.Warn("Bitcoin address network mismatch", zap.String("address", address), zap.String("expected_network", network))
		return false, nil
	}

	return true, nil
}

// ImportWalletFromWIF imports an existing wallet from a WIF (Wallet Import Format) private key.
// Used during card redemption: decrypt WIF from database, import wallet, sign transaction.
func ImportWalletFromWIF(wif string, network string) (*Wallet, error) {
	// 1. Validate network parameter
	if network != "mainnet" && network != "testnet" {
		return nil, errors.New("invalid network: must be 'mainnet' or 'testnet'")
	}

	params := getNetworkConfig(network)

	// 3. Decode WIF to extract private key
	privKeyWif, err := btcutil.DecodeWIF(wif)
	if err != nil {
		logger.Error("Failed to decode WIF", zap.Error(err))
		return nil, err
	}

	// 4. Verify WIF network matches requested network
	if !privKeyWif.IsForNet(params) {
		logger.Error("WIF network mismatch",
			zap.String("expected", network),
			zap.Bool("is_mainnet", privKeyWif.IsForNet(&chaincfg.MainNetParams)))
		return nil, errors.New("WIF network does not match requested network")
	}

	// 5. Extract private key and derive public key
	privKey := privKeyWif.PrivKey
	publicKey := privKey.PubKey()

	// 6. Generate SegWit address from public key (same as GenerateWallet)
	pubKeyHash := btcutil.Hash160(publicKey.SerializeCompressed())
	address, err := btcutil.NewAddressWitnessPubKeyHash(pubKeyHash, params)
	if err != nil {
		logger.Error("Failed to regenerate address from WIF", zap.Error(err))
		return nil, err
	}

	// 7. Return Wallet struct (identical structure to GenerateWallet)
	return &Wallet{
		PrivateKey: privKeyWif.String(),
		PublicKey:  publicKey.SerializeCompressed(),
		Address:    address.EncodeAddress(),
		Network:    network,
	}, nil
}

// GetUTXOs fetches unspent transaction outputs for the wallet from Blockstream API.
// Returns empty slice if no UTXOs are available.
func (w *Wallet) GetUTXOs() ([]UTXO, error) {
	// Determine API URL based on w.Network
	var apiUrl string
	if w.Network == "mainnet" {
		apiUrl = "https://blockstream.info/api/address/" + w.Address + "/utxo"
	} else {
		apiUrl = "https://blockstream.info/testnet/api/address/" + w.Address + "/utxo"
	}

	// Make HTTP GET request
	resp, err := http.Get(apiUrl)
	if err != nil {
		logger.Error("Failed to fetch UTXOs", zap.Error(err))
		return nil, err
	}
	defer resp.Body.Close()

	// Check HTTP status
	if resp.StatusCode != 200 {
		logger.Error("API returned error", zap.Int("status", resp.StatusCode))
		return nil, fmt.Errorf("API error: status %d", resp.StatusCode)
	}

	// Parse JSON response
	var utxos []UTXO
	err = json.NewDecoder(resp.Body).Decode(&utxos)
	if err != nil {
		logger.Error("Failed to parse UTXO response", zap.Error(err))
		return nil, err
	}

	return utxos, nil
}

// selectCoins performs coin selection from available UTXOs
// Returns selected UTXOs, total input amount, and change amount
func selectCoins(utxos []UTXO, amount btcutil.Amount, feeRate int64) ([]UTXO, btcutil.Amount, btcutil.Amount, error) {
	var selectedUTXOs []UTXO
	var totalInput btcutil.Amount
	numOutputs := 2 // Assume change output initially

	// Progressive coin selection
	for _, utxo := range utxos {
		// Only use confirmed UTXOs
		if !utxo.Status.Confirmed {
			continue
		}

		// Add this UTXO to selection
		selectedUTXOs = append(selectedUTXOs, utxo)
		totalInput += btcutil.Amount(utxo.Value)

		// Recalculate fee based on current input count
		numInputs := len(selectedUTXOs)
		txSize := int64((numInputs * 68) + (numOutputs * 31) + 11)
		fee := btcutil.Amount(txSize * feeRate)
		totalNeeded := amount + fee

		// Check if we have enough
		if totalInput >= totalNeeded {
			// Calculate change
			change := totalInput - totalNeeded

			// If change is dust (< 546 sats), add it to fee
			if change < 546 {
				change = 0
			}

			// Return successful selection
			return selectedUTXOs, totalInput, change, nil
		}
	}

	// Not enough funds
	return nil, 0, 0, fmt.Errorf("insufficient funds: have %d sats, need %d sats",
		totalInput, amount)
}

// Before redemption: Verify card has funds
// TODO put urls in config with env variables
func (w *Wallet) GetBalance() (btcutil.Amount, error) {
	// Fetch UTXOs
	utxos, err := w.GetUTXOs()
	if err != nil {
		logger.Error("Failed to fetch UTXOs", zap.Error(err))
		return 0, err
	}

	// Sum all UTXO values
	var balance int64
	for _, utxo := range utxos {
		if utxo.Status.Confirmed { // Only count confirmed UTXOs
			balance += utxo.Value
		}
	}

	// Return as btcutil.Amount
	return btcutil.Amount(balance), nil
}

// Main redemption logic: Send BTC to user's address
func (w *Wallet) CreateTransaction(toAddress string, amount btcutil.Amount, feeRate int64) (*wire.MsgTx, error) {
	// Validate inputs
	valid, err := ValidateAddress(toAddress, w.Network)
	if err != nil {
		logger.Error("Failed address validation", zap.String("address", toAddress), zap.Error(err))
		return nil, err
	}
	if !valid {
		return nil, errors.New("invalid destination address")
	}

	if amount <= 0 {
		return nil, fmt.Errorf("Invalid amount to send %d", amount)
	}

	if feeRate <= 0 {
		return nil, fmt.Errorf("Invalid fee rate %d", feeRate)
	}

	// Fetch UTXOs
	utxos, err := w.GetUTXOs()
	if err != nil {
		logger.Error("Failed to fetch UTXOs", zap.Error(err))
		return nil, err
	}

	// Perform coin selection
	selectedUTXOs, _, change, err := selectCoins(utxos, amount, feeRate)
	if err != nil {
		return nil, err
	}

	// Create transaction
	// Get network params
	params := getNetworkConfig(w.Network)

	// Create new transaction
	tx := wire.NewMsgTx(wire.TxVersion)

	// Add inputs from selected UTXOs
	for _, utxo := range selectedUTXOs {
		txHash, err := chainhash.NewHashFromStr(utxo.TxHash)
		if err != nil {
			return nil, fmt.Errorf("invalid tx hash: %v", err)
		}

		outPoint := wire.NewOutPoint(txHash, utxo.Vout)
		txIn := wire.NewTxIn(outPoint, nil, nil)
		tx.AddTxIn(txIn)
	}

	// Add output to recipient
	toAddr, err := btcutil.DecodeAddress(toAddress, params)
	if err != nil {
		return nil, fmt.Errorf("failed to decode recipient address: %v", err)
	}
	pkScript, err := txscript.PayToAddrScript(toAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to create output script: %v", err)
	}
	tx.AddTxOut(wire.NewTxOut(int64(amount), pkScript))

	// Add change output if needed (change was calculated in selectCoins)
	if change > 546 {
		changeAddr, err := btcutil.DecodeAddress(w.Address, params)
		if err != nil {
			return nil, fmt.Errorf("failed to decode change address: %v", err)
		}
		changePkScript, err := txscript.PayToAddrScript(changeAddr)
		if err != nil {
			return nil, fmt.Errorf("failed to create change script: %v", err)
		}
		tx.AddTxOut(wire.NewTxOut(int64(change), changePkScript))
	}

	return tx, nil

}

// Sign the transaction so it can be broadcast
func (w *Wallet) SignTransaction(tx *wire.MsgTx, utxos []UTXO) (*wire.MsgTx, error) {
	// Decode WIF to extract private key
	privKeyWif, err := btcutil.DecodeWIF(w.PrivateKey)
	if err != nil {
		logger.Error("Failed to decode WIF", zap.Error(err))
		return nil, err
	}

	privKey := privKeyWif.PrivKey

	// Get network parameters
	params := getNetworkConfig(w.Network)

	for i, txIn := range tx.TxIn {
		// Get corresponding UTXO for this input
		utxo := utxos[i]

		// Create signature hash
		sigHashes := txscript.NewTxSigHashes(tx, nil)

		// Create witness script (P2WPKH)
		witnessPubKeyHash := btcutil.Hash160(w.PublicKey)
		witnessAddr, err := btcutil.NewAddressWitnessPubKeyHash(witnessPubKeyHash, params)
		if err != nil {
			return nil, fmt.Errorf("failed to create witness address: %v", err)
		}
		witnessScript, err := txscript.PayToAddrScript(witnessAddr)
		if err != nil {
			return nil, fmt.Errorf("failed to create witness script: %v", err)
		}

		// Sign the transaction
		signature, err := txscript.RawTxInWitnessSignature(
			tx, sigHashes, i, utxo.Value,
			witnessScript, txscript.SigHashAll, privKey)
		if err != nil {
			return nil, fmt.Errorf("failed to sign input %d: %v", i, err)
		}

		// Add witness data (signature + public key)
		txIn.Witness = wire.TxWitness{signature, w.PublicKey}
	}

	return tx, nil
}

// Submit to mempool for confirmation
func (w *Wallet) BroadcastTransaction(signedTx *wire.MsgTx) (string, error) {
	// Serialize transaction to hex
	var buf bytes.Buffer
	err := signedTx.Serialize(&buf)
	if err != nil {
		return "", fmt.Errorf("failed to serialize transaction: %v", err)
	}

	txHex := hex.EncodeToString(buf.Bytes())

	// Determine API URL based on network
	var url string
	if w.Network == "mainnet" {
		url = "https://blockstream.info/api/tx"
	} else {
		url = "https://blockstream.info/testnet/api/tx"
	}

	// Broadcast transaction
	resp, err := http.Post(url, "text/plain", strings.NewReader(txHex))
	if err != nil {
		return "", fmt.Errorf("failed to broadcast transaction: %v", err)
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %v", err)
	}

	// Check HTTP status
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("broadcast failed: %s", string(body))
	}

	// Return transaction ID
	txid := signedTx.TxHash().String()
	logger.Info("Transaction broadcasted",
		zap.String("txid", txid),
		zap.String("network", w.Network))
	return txid, nil
}
