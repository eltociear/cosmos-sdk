package keyring_test

import (
	"encoding/hex"
	"fmt"
	"strings"
	"testing"

	keyring99designs "github.com/99designs/keyring"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	bip39 "github.com/cosmos/go-bip39"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/cosmos-sdk/crypto"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	"github.com/cosmos/cosmos-sdk/crypto/keys/multisig"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	"github.com/cosmos/cosmos-sdk/crypto/types"
	"github.com/cosmos/cosmos-sdk/simapp"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

const (
	someKey = "theKey"
	theID   = "theID"
	otherID = "otherID"
)

func init() {
	crypto.BcryptSecurityParameter = 1
}


func TestNewKeyring(t *testing.T) {
	dir := t.TempDir()
	mockIn := strings.NewReader("")
	encCfg := simapp.MakeTestEncodingConfig()

	kr, err := keyring.New("cosmos", keyring.BackendFile, dir, mockIn, encCfg.Marshaler)
	require.NoError(t, err)

	nilKr, err := keyring.New("cosmos", "fuzzy", dir, mockIn, encCfg.Marshaler)
	require.Error(t, err)
	require.Nil(t, nilKr)
	require.Equal(t, "unknown keyring backend fuzzy", err.Error())

	mockIn.Reset("password\npassword\n")
	ke, _, err := kr.NewMnemonic("foo", keyring.English, sdk.FullFundraiserPath, keyring.DefaultBIP39Passphrase, hd.Secp256k1)
	require.NoError(t, err)
	require.Equal(t, "foo", ke.GetName())
}

func TestKeyManagementKeyRing(t *testing.T) {
	encCfg := simapp.MakeTestEncodingConfig()
	kb, err := keyring.New("keybasename", "test", t.TempDir(), nil, encCfg.Marshaler)
	require.NoError(t, err)

	algo := hd.Secp256k1
	n1, n2, n3 := "personal", "business", "other"

	// Check empty state
	l, err := kb.List()
	require.Nil(t, err)
	require.Empty(t, l)

	_, _, err = kb.NewMnemonic(n1, keyring.English, sdk.FullFundraiserPath, keyring.DefaultBIP39Passphrase, keyring.NotSupportedAlgo{})
	require.Error(t, err, "ed25519 keys are currently not supported by keybase")

	// create some keys
	_, err = kb.Key(n1)
	require.Error(t, err)
	ke, _, err := kb.NewMnemonic(n1, keyring.English, sdk.FullFundraiserPath, keyring.DefaultBIP39Passphrase, algo)

	require.NoError(t, err)
	require.Equal(t, n1, ke.GetName())
	_, _, err = kb.NewMnemonic(n2, keyring.English, sdk.FullFundraiserPath, keyring.DefaultBIP39Passphrase, algo)
	require.NoError(t, err)

	// we can get these keys
	i2, err := kb.Key(n2)
	require.NoError(t, err)
	_, err = kb.Key(n3)
	require.NotNil(t, err)
	addr, err := accAddr(i2)
	require.NoError(t, err)
	_, err = kb.KeyByAddress(addr)
	require.NoError(t, err)
	addr, err = sdk.AccAddressFromBech32("cosmos1yq8lgssgxlx9smjhes6ryjasmqmd3ts2559g0t")
	require.NoError(t, err)
	_, err = kb.KeyByAddress(addr)
	require.NotNil(t, err)

	// list shows them in order
	keyS, err := kb.List()
	require.NoError(t, err)
	require.Equal(t, 2, len(keyS))
	// note these are in alphabetical order
	require.Equal(t, n2, keyS[0].GetName())
	require.Equal(t, n1, keyS[1].GetName())

	key1, err := i2.GetPubKey()
	require.NoError(t, err)
	require.NotNil(t, key1)
	key2, err := keyS[0].GetPubKey()
	require.NoError(t, err)
	require.NotNil(t, key2)
	require.Equal(t, key1, key2)

	// deleting a key removes it
	err = kb.Delete("bad name")
	require.NotNil(t, err)
	err = kb.Delete(n1)
	require.NoError(t, err)
	keyS, err = kb.List()
	require.NoError(t, err)
	require.Equal(t, 1, len(keyS))
	_, err = kb.Key(n1)
	require.Error(t, err)

	// create an offline key
	o1 := "offline"
	priv1 := ed25519.GenPrivKey()
	pub1 := priv1.PubKey()
	ke, err = kb.SavePubKey(o1, pub1, hd.Ed25519Type)
	require.Nil(t, err)

	key1, err = ke.GetPubKey()
	require.NoError(t, err)
	require.NotNil(t, key1)
	require.Equal(t, pub1, key1)

	require.Equal(t, o1, i.GetName())
	keyS, err = kb.List()
	require.NoError(t, err)
	require.Equal(t, 2, len(keyS))

	// delete the offline key
	err = kb.Delete(o1)
	require.NoError(t, err)
	keyS, err = kb.List()
	require.NoError(t, err)
	require.Equal(t, 1, len(keyS))

	// addr cache gets nuked - and test skip flag
	require.NoError(t, kb.Delete(n2))
}

func TestSignVerifyKeyRing(t *testing.T) {
	dir := t.TempDir()
	encCfg := simapp.MakeTestEncodingConfig()

	kb, err := keyring.New("keybasename", "test", dir, nil, encCfg.Marshaler)
	require.NoError(t, err)
	algo := hd.Secp256k1

	n1, n2, n3 := "some dude", "a dudette", "dude-ish"

	// create two users and get their info
	ke1, _, err := kb.NewMnemonic(n1, keyring.English, sdk.FullFundraiserPath, keyring.DefaultBIP39Passphrase, algo)
	require.Nil(t, err)

	ke2, _, err := kb.NewMnemonic(n2, keyring.English, sdk.FullFundraiserPath, keyring.DefaultBIP39Passphrase, algo)
	require.Nil(t, err)

	// let's try to sign some messages
	d1 := []byte("my first message")
	d2 := []byte("some other important info!")
	d3 := []byte("feels like I forgot something...")

	// try signing both data with both ..
	s11, pub1, err := kb.Sign(n1, d1)
	require.Nil(t, err)

	key1, err := ke1.GetPubKey()
	require.NoError(t, err)
	require.NotNil(t, key1)
	require.Equal(t, key1, pub1)

	s12, pub1, err := kb.Sign(n1, d2)
	require.Nil(t, err)
	require.Equal(t, key1, pub1)

	s21, pub2, err := kb.Sign(n2, d1)
	require.Nil(t, err)
	key2, err := ke2.GetPubKey()
	require.NoError(t, err)
	require.NotNil(t, key2)
	require.Equal(t, key2, pub2)

	s22, pub2, err := kb.Sign(n2, d2)
	require.Nil(t, err)
	require.Equal(t, key2, pub2)

	// let's try to validate and make sure it only works when everything is proper
	cases := []struct {
		key   types.PubKey
		data  []byte
		sig   []byte
		valid bool
	}{
		// proper matches
		{key1, d1, s11, true},
		// change data, pubkey, or signature leads to fail
		{key1, d2, s11, false},
		{key2, d1, s11, false},
		{key1, d1, s21, false},
		// make sure other successes
		{key1, d2, s12, true},
		{key2, d1, s21, true},
		{key2, d2, s22, true},
	}

	for i, tc := range cases {
		valid := tc.key.VerifySignature(tc.data, tc.sig)
		require.Equal(t, tc.valid, valid, "%d", i)
	}

	// Now try to sign data with a secret-less key
	// Import a public key
	armor, err := kb.ExportPubKeyArmor(n2)
	require.NoError(t, err)
	require.NoError(t, kb.Delete(n2))

	require.NoError(t, kb.ImportPubKey(n3, armor))
	i3, err := kb.Key(n3)
	require.NoError(t, err)
	require.Equal(t, i3.GetName(), n3)

	_, _, err = kb.Sign(n3, d3)
	require.Error(t, err)
	require.Equal(t, "cannot sign with offline keys", err.Error())
}

func TestExportImportKeyRing(t *testing.T) {
	encCfg := simapp.MakeTestEncodingConfig()
	kb, err := keyring.New("keybasename", "test", t.TempDir(), nil, encCfg.Marshaler)
	require.NoError(t, err)

	ke, _, err := kb.NewMnemonic("john", keyring.English, sdk.FullFundraiserPath, keyring.DefaultBIP39Passphrase, hd.Secp256k1)
	name := ke.GetName()
	require.NoError(t, err)
	require.Equal(t, name, "john")

	john, err := kb.Key("john")
	require.NoError(t, err)
	require.Equal(t, name, "john")
	key, err := ke.GetPubKey()
	require.NoError(t, err)
	johnAddr := key.Address()

	armor, err := kb.ExportPrivKeyArmor("john", "apassphrase")
	require.NoError(t, err)
	err = kb.Delete("john")
	require.NoError(t, err)

	err = kb.ImportPrivKey("john2", armor, "apassphrase")
	require.NoError(t, err)

	john2, err := kb.Key("john2")
	require.NoError(t, err)

	require.Equal(t, key.Address(), johnAddr)
	require.Equal(t, john.GetName(), "john")

	addr, err := john.GetAddress()
	require.NoError(t, err)
	addr2, err := john2.GetAddress()
	require.NoError(t, err)
	require.Equal(t, addr, addr2)

	require.Equal(t, john.GetAlgo(), john2.GetAlgo())

	key, err = john.GetPubKey()
	require.NoError(t, err)
	key2, err := john2.GetPubKey()
	require.NoError(t, err)

	require.Equal(t, key, key2)

	//TODO do we require GetType or pubKeyType is sufficient?
	//	require.Equal(t, john.GetType(), john2.GetType())
}

func TestExportImportPubKeyKeyRing(t *testing.T) {
	encCfg := simapp.MakeTestEncodingConfig()
	kb, err := keyring.New("keybasename", "test", t.TempDir(), nil, encCfg.Marshaler)
	require.NoError(t, err)
	algo := hd.Secp256k1

	// CreateMnemonic a private-public key pair and ensure consistency
	ke, _, err := kb.NewMnemonic("john", keyring.English, sdk.FullFundraiserPath, keyring.DefaultBIP39Passphrase, algo)
	require.Nil(t, err)
	require.NotEqual(t, ke, "")
	require.Equal(t, ke.GetName(), "john")
	key, err := ke.GetPubKey()
	require.NoError(t, err)
	addr := key.Address()
	john, err := kb.Key("john")
	require.NoError(t, err)
	require.Equal(t, john.GetName(), "john")

	key, err = john.GetPubKey()
	require.NoError(t, err)
	require.Equal(t, key.Address(), addr)

	// Export the public key only
	armor, err := kb.ExportPubKeyArmor("john")
	require.NoError(t, err)
	err = kb.Delete("john")
	require.NoError(t, err)

	// Import it under a different name
	err = kb.ImportPubKey("john-pubkey-only", armor)
	require.NoError(t, err)

	// Ensure consistency
	john2, err := kb.Key("john-pubkey-only")
	require.NoError(t, err)
	key2, err := john2.GetPubKey()
	require.NoError(t, err)

	// Compare the public keys
	require.True(t, key.Equals(key2))

	// Ensure keys cannot be overwritten
	err = kb.ImportPubKey("john-pubkey-only", armor)
	require.NotNil(t, err)
}

func TestAdvancedKeyManagementKeyRing(t *testing.T) {
	dir := t.TempDir()
	encCfg := simapp.MakeTestEncodingConfig()

	kb, err := keyring.New("keybasename", "test", dir, nil, encCfg.Marshaler)
	require.NoError(t, err)

	algo := hd.Secp256k1
	n1, n2 := "old-name", "new name"

	// make sure key works with initial password
	_, _, err = kb.NewMnemonic(n1, keyring.English, sdk.FullFundraiserPath, keyring.DefaultBIP39Passphrase, algo)
	require.Nil(t, err, "%+v", err)

	_, err = kb.ExportPubKeyArmor(n1 + ".notreal")
	require.NotNil(t, err)
	_, err = kb.ExportPubKeyArmor(" " + n1)
	require.NotNil(t, err)
	_, err = kb.ExportPubKeyArmor(n1 + " ")
	require.NotNil(t, err)
	_, err = kb.ExportPubKeyArmor("")
	require.NotNil(t, err)
	exported, err := kb.ExportPubKeyArmor(n1)
	require.Nil(t, err, "%+v", err)
	err = kb.Delete(n1)
	require.NoError(t, err)

	// import succeeds
	err = kb.ImportPubKey(n2, exported)
	require.NoError(t, err)

	// second import fails
	err = kb.ImportPubKey(n2, exported)
	require.NotNil(t, err)
}

func TestSeedPhraseKeyRing(t *testing.T) {
	dir := t.TempDir()
	encCfg := simapp.MakeTestEncodingConfig()

	kb, err := keyring.New("keybasename", "test", dir, nil, encCfg.Marshaler)
	require.NoError(t, err)

	algo := hd.Secp256k1
	n1, n2 := "lost-key", "found-again"

	// make sure key works with initial password
	ke, mnemonic, err := kb.NewMnemonic(n1, keyring.English, sdk.FullFundraiserPath, keyring.DefaultBIP39Passphrase, algo)
	require.Nil(t, err, "%+v", err)
	require.Equal(t, n1, ke.GetName())
	require.NotEmpty(t, mnemonic)
	key, err := ke.GetPubKey()
	require.NoError(t, err)

	// now, let us delete this key
	err = kb.Delete(n1)
	require.Nil(t, err, "%+v", err)
	_, err = kb.Key(n1)
	require.NotNil(t, err)

	// let us re-create it from the mnemonic-phrase
	params := *hd.NewFundraiserParams(0, sdk.CoinType, 0)
	hdPath := params.String()
	newKe, err := kb.NewAccount(n2, mnemonic, keyring.DefaultBIP39Passphrase, hdPath, hd.Secp256k1)
	require.NoError(t, err)
	require.Equal(t, n2, newKe.GetName())
	newKey, err := newKe.GetPubKey()
	require.NoError(t, err)

	require.Equal(t, key.Address(), newKey.Address())
	require.Equal(t, key, newKey)
}

func TestKeyringKeybaseExportImportPrivKey(t *testing.T) {
	encCfg := simapp.MakeTestEncodingConfig()
	kb, err := keyring.New("keybasename", "test", t.TempDir(), nil, encCfg.Marshaler)
	require.NoError(t, err)

	_, _, err = kb.NewMnemonic("john", keyring.English, sdk.FullFundraiserPath, keyring.DefaultBIP39Passphrase, hd.Secp256k1)
	require.NoError(t, err)

	keystr, err := kb.ExportPrivKeyArmor("john", "somepassword")
	require.NoError(t, err)
	require.NotEmpty(t, keystr)
	err = kb.Delete("john")
	require.NoError(t, err)

	// try import the key - wrong password
	err = kb.ImportPrivKey("john2", keystr, "bad pass")
	require.Equal(t, "failed to decrypt private key: ciphertext decryption failed", err.Error())

	// try import the key with the correct password
	require.NoError(t, kb.ImportPrivKey("john2", keystr, "somepassword"))

	// overwrite is not allowed
	err = kb.ImportPrivKey("john2", keystr, "password")
	require.Equal(t, "cannot overwrite key: john2", err.Error())

	// try export non existing key
	_, err = kb.ExportPrivKeyArmor("john3", "wrongpassword")
	require.EqualError(t, err, "john3.info: key not found")
}

func TestInMemoryLanguage(t *testing.T) {
	encCfg := simapp.MakeTestEncodingConfig()
	kb := keyring.NewInMemory(encCfg.Marshaler)
	_, _, err := kb.NewMnemonic("something", keyring.Japanese, sdk.FullFundraiserPath, keyring.DefaultBIP39Passphrase, hd.Secp256k1)
	require.Error(t, err)
	require.Equal(t, "unsupported language: only keyring.English is supported", err.Error())
}

func TestInMemoryCreateMultisig(t *testing.T) {
	encCfg := simapp.MakeTestEncodingConfig()
	kb, err := keyring.New("keybasename", "memory", "", nil, encCfg.Marshaler)
	require.NoError(t, err)
	multi := multisig.NewLegacyAminoPubKey(
		1, []types.PubKey{
			secp256k1.GenPrivKey().PubKey(),
		},
	)
	_, err = kb.SaveMultisig("multi", multi)
	require.NoError(t, err)
}

func TestInMemoryCreateAccountInvalidMnemonic(t *testing.T) {
	encCfg := simapp.MakeTestEncodingConfig()
	kb := keyring.NewInMemory(encCfg.Marshaler)
	_, err := kb.NewAccount(
		"some_account",
		"malarkey pair crucial catch public canyon evil outer stage ten gym tornado",
		"", hd.CreateHDPath(118, 0, 0).String(), hd.Secp256k1)
	require.Error(t, err)
	require.Equal(t, "Invalid mnemonic", err.Error())
}

// TestInMemoryKeyManagement makes sure we can manipulate these keys well
func TestInMemoryKeyManagement(t *testing.T) {
	// make the storage with reasonable defaults
	encCfg := simapp.MakeTestEncodingConfig()
	cstore := keyring.NewInMemory(encCfg.Marshaler)

	algo := hd.Secp256k1
	n1, n2, n3 := "personal", "business", "other"

	// Check empty state
	l, err := cstore.List()
	require.Nil(t, err)
	require.Empty(t, l)

	_, _, err = cstore.NewMnemonic(n1, keyring.English, sdk.FullFundraiserPath, keyring.DefaultBIP39Passphrase, keyring.NotSupportedAlgo{})
	require.Error(t, err, "ed25519 keys are currently not supported by keybase")

	// create some keys
	_, err = cstore.Key(n1)
	require.Error(t, err)
	ke, _, err := cstore.NewMnemonic(n1, keyring.English, sdk.FullFundraiserPath, keyring.DefaultBIP39Passphrase, algo)

	require.NoError(t, err)
	require.Equal(t, n1, i.GetName())
	_, _, err = cstore.NewMnemonic(n2, keyring.English, sdk.FullFundraiserPath, keyring.DefaultBIP39Passphrase, algo)
	require.NoError(t, err)

	// we can get these keys
	ke2, err := cstore.Key(n2)
	require.NoError(t, err)
	_, err = cstore.Key(n3)
	require.NotNil(t, err)
	addr, err := accAddr(ke2)
	require.NoError(t, err)
	_, err = cstore.KeyByAddress(addr)
	require.NoError(t, err)
	addr, err = sdk.AccAddressFromBech32("cosmos1yq8lgssgxlx9smjhes6ryjasmqmd3ts2559g0t")
	require.NoError(t, err)
	_, err = cstore.KeyByAddress(addr)
	require.NotNil(t, err)

	// list shows them in order
	keyS, err := cstore.List()
	require.NoError(t, err)
	require.Equal(t, 2, len(keyS))
	// note these are in alphabetical order
	require.Equal(t, n2, keyS[0].GetName())
	require.Equal(t, n1, keyS[1].GetName())

	key1, err := ke2.GetPubKey()
	require.NoError(t, err)
	key2, err := keyS[0].GetPubKey()
	require.NoError(t, err)

	require.Equal(t, key1, key2)

	// deleting a key removes it
	err = cstore.Delete("bad name")
	require.NotNil(t, err)
	err = cstore.Delete(n1)
	require.NoError(t, err)
	keyS, err = cstore.List()
	require.NoError(t, err)
	require.Equal(t, 1, len(keyS))
	_, err = cstore.Key(n1)
	require.Error(t, err)

	// create an offline key
	o1 := "offline"
	priv1 := ed25519.GenPrivKey()
	pub1 := priv1.PubKey()
	ke, err = cstore.SavePubKey(o1, pub1, hd.Ed25519Type)
	require.Nil(t, err)

	key, err := ke.GetPubKey()
	require.NoError(t, err)
	require.Equal(t, pub1, key)

	require.Equal(t, o1, ke.GetName())
	require.NotNil(t, ke.GetOffline())
	require.Equal(t, hd.Ed25519Type, ke.GetAlgo())
	keyS, err = cstore.List()
	require.NoError(t, err)
	require.Equal(t, 2, len(keyS))

	// delete the offline key
	err = cstore.Delete(o1)
	require.NoError(t, err)
	keyS, err = cstore.List()
	require.NoError(t, err)
	require.Equal(t, 1, len(keyS))

	// addr cache gets nuked - and test skip flag
	err = cstore.Delete(n2)
	require.NoError(t, err)
}

// TestInMemorySignVerify does some detailed checks on how we sign and validate
// signatures
func TestInMemorySignVerify(t *testing.T) {
	encCfg := simapp.MakeTestEncodingConfig()
	cstore := keyring.NewInMemory(encCfg.Marshaler)
	algo := hd.Secp256k1

	n1, n2, n3 := "some dude", "a dudette", "dude-ish"

	// create two users and get their info
	ke1, _, err := cstore.NewMnemonic(n1, keyring.English, sdk.FullFundraiserPath, keyring.DefaultBIP39Passphrase, algo)
	require.Nil(t, err)

	ke2, _, err := cstore.NewMnemonic(n2, keyring.English, sdk.FullFundraiserPath, keyring.DefaultBIP39Passphrase, algo)
	require.Nil(t, err)

	// let's try to sign some messages
	d1 := []byte("my first message")
	d2 := []byte("some other important info!")
	d3 := []byte("feels like I forgot something...")

	// try signing both data with both ..
	s11, pub1, err := cstore.Sign(n1, d1)
	require.Nil(t, err)
	key1, err := ke1.GetPubKey()
	require.NoError(t, err)
	require.Equal(t, key1, pub1)

	s12, pub1, err := cstore.Sign(n1, d2)
	require.Nil(t, err)
	require.Equal(t, key1, pub1)

	s21, pub2, err := cstore.Sign(n2, d1)
	require.Nil(t, err)
	key2, err := ke2.GetPubKey()
	require.NoError(t, err)
	require.Equal(t, key2, pub2)

	s22, pub2, err := cstore.Sign(n2, d2)
	require.Nil(t, err)
	require.Equal(t, key2, pub2)

	// let's try to validate and make sure it only works when everything is proper
	cases := []struct {
		key   types.PubKey
		data  []byte
		sig   []byte
		valid bool
	}{
		// proper matches
		{key1, d1, s11, true},
		// change data, pubkey, or signature leads to fail
		{key1, d2, s11, false},
		{key2, d1, s11, false},
		{key1, d1, s21, false},
		// make sure other successes
		{key1, d2, s12, true},
		{key2, d1, s21, true},
		{key2, d2, s22, true},
	}

	for i, tc := range cases {
		valid := tc.key.VerifySignature(tc.data, tc.sig)
		require.Equal(t, tc.valid, valid, "%d", i)
	}

	// Import a public key
	armor, err := cstore.ExportPubKeyArmor(n2)
	require.Nil(t, err)
	err = cstore.Delete(n2)
	require.NoError(t, err)
	err = cstore.ImportPubKey(n3, armor)
	require.NoError(t, err)
	i3, err := cstore.Key(n3)
	require.NoError(t, err)
	require.Equal(t, i3.GetName(), n3)

	// Now try to sign data with a secret-less key
	_, _, err = cstore.Sign(n3, d3)
	require.Error(t, err)
	require.Equal(t, "cannot sign with offline keys", err.Error())
}

// TestInMemoryExportImport tests exporting and importing
func TestInMemoryExportImport(t *testing.T) {
	// make the storage with reasonable defaults
	encCfg := simapp.MakeTestEncodingConfig()
	cstore := keyring.NewInMemory(encCfg.Marshaler)

	ke, _, err := cstore.NewMnemonic("john", keyring.English, sdk.FullFundraiserPath, keyring.DefaultBIP39Passphrase, hd.Secp256k1)
	require.NoError(t, err)
	require.Equal(t, ke.GetName(), "john")

	john, err := cstore.Key("john")
	require.NoError(t, err)
	require.Equal(t, ke.GetName(), "john")
	johnKey, err := ke.GetPubKey()
	require.NoError(t, err)
	johnAddr := johnKey.Address()

	armor, err := cstore.ExportPubKeyArmor("john")
	require.NoError(t, err)
	err = cstore.Delete("john")
	require.NoError(t, err)

	err = cstore.ImportPubKey("john2", armor)
	require.NoError(t, err)

	john2, err := cstore.Key("john2")
	require.NoError(t, err)

	require.Equal(t, johnKey.Address(), johnAddr)
	require.Equal(t, john.GetName(), "john")

	johnSdkAddress, err := john.GetAddress()
	require.NoError(t, err)
	john2SdkAddress, err := john2.GetAddress()
	require.NoError(t, err)
	require.Equal(t, johnSdkAddress, john2SdkAddress)

	require.Equal(t, john.GetAlgo(), john2.GetAlgo())

	john2Key, err := john2.GetPubKey()
	require.NoError(t, err)

	require.Equal(t, johnKey, john2Key)
}

func TestInMemoryExportImportPrivKey(t *testing.T) {
	encCfg := simapp.MakeTestEncodingConfig()
	kb := keyring.NewInMemory(encCfg.Marshaler)

	ke, _, err := kb.NewMnemonic("john", keyring.English, sdk.FullFundraiserPath, keyring.DefaultBIP39Passphrase, hd.Secp256k1)
	require.NoError(t, err)
	require.Equal(t, ke.GetName(), "john")
	priv1, err := kb.Key("john")
	require.NoError(t, err)

	armored, err := kb.ExportPrivKeyArmor("john", "secretcpw")
	require.NoError(t, err)

	// delete exported key
	require.NoError(t, kb.Delete("john"))
	_, err = kb.Key("john")
	require.Error(t, err)

	// import armored key
	require.NoError(t, kb.ImportPrivKey("john", armored, "secretcpw"))

	// ensure old and new keys match
	priv2, err := kb.Key("john")
	require.NoError(t, err)
	key1, err := priv1.GetPubKey()
	require.NoError(t, err)
	key2, err := priv2.GetPubKey()
	require.NoError(t, err)
	require.True(t, key1.Equals(key2))
}

func TestInMemoryExportImportPubKey(t *testing.T) {
	// make the storage with reasonable defaults
	encCfg := simapp.MakeTestEncodingConfig()
	cstore := keyring.NewInMemory(encCfg.Marshaler)

	// CreateMnemonic a private-public key pair and ensure consistency
	ke, _, err := cstore.NewMnemonic("john", keyring.English, sdk.FullFundraiserPath, keyring.DefaultBIP39Passphrase, hd.Secp256k1)
	require.Nil(t, err)
	require.NotEqual(t, info, "")
	require.Equal(t, ke.GetName(), "john")
	key, err := ke.GetPubKey()
	require.NoError(t, err)
	addr := key.Address()
	john, err := cstore.Key("john")
	require.NoError(t, err)
	require.Equal(t, john.GetName(), "john")
	johnKey, err := john.GetPubKey()
	require.NoError(t, err)
	require.Equal(t, johnKey.Address(), addr)

	// Export the public key only
	armor, err := cstore.ExportPubKeyArmor("john")
	require.NoError(t, err)
	err = cstore.Delete("john")
	require.NoError(t, err)

	// Import it under a different name
	err = cstore.ImportPubKey("john-pubkey-only", armor)
	require.NoError(t, err)
	// Ensure consistency
	john2, err := cstore.Key("john-pubkey-only")
	require.NoError(t, err)
	// Compare the public keys
	john2Key, err := john2.GetPubKey()
	require.NoError(t, err)
	require.True(t, johnKey.Equals(john2Key))

	// Ensure keys cannot be overwritten
	err = cstore.ImportPubKey("john-pubkey-only", armor)
	require.NotNil(t, err)
}

// TestInMemoryAdvancedKeyManagement verifies update, import, export functionality
func TestInMemoryAdvancedKeyManagement(t *testing.T) {
	// make the storage with reasonable defaults
	encCfg := simapp.MakeTestEncodingConfig()
	cstore := keyring.NewInMemory(encCfg.Marshaler)

	algo := hd.Secp256k1
	n1, n2 := "old-name", "new name"

	// make sure key works with initial password
	_, _, err := cstore.NewMnemonic(n1, keyring.English, sdk.FullFundraiserPath, keyring.DefaultBIP39Passphrase, algo)
	require.Nil(t, err, "%+v", err)

	// exporting requires the proper name and passphrase
	_, err = cstore.ExportPubKeyArmor(n1 + ".notreal")
	require.NotNil(t, err)
	_, err = cstore.ExportPubKeyArmor(" " + n1)
	require.NotNil(t, err)
	_, err = cstore.ExportPubKeyArmor(n1 + " ")
	require.NotNil(t, err)
	_, err = cstore.ExportPubKeyArmor("")
	require.NotNil(t, err)
	exported, err := cstore.ExportPubKeyArmor(n1)
	require.Nil(t, err, "%+v", err)
	err = cstore.Delete(n1)
	require.NoError(t, err)

	// import succeeds
	err = cstore.ImportPubKey(n2, exported)
	require.NoError(t, err)

	// second import fails
	err = cstore.ImportPubKey(n2, exported)
	require.NotNil(t, err)
}

// TestInMemorySeedPhrase verifies restoring from a seed phrase
func TestInMemorySeedPhrase(t *testing.T) {
	// make the storage with reasonable defaults
	encCfg := simapp.MakeTestEncodingConfig()
	cstore := keyring.NewInMemory(encCfg.Marshaler)

	algo := hd.Secp256k1
	n1, n2 := "lost-key", "found-again"

	// make sure key works with initial password
	info, mnemonic, err := cstore.NewMnemonic(n1, keyring.English, sdk.FullFundraiserPath, keyring.DefaultBIP39Passphrase, algo)
	require.Nil(t, err, "%+v", err)
	require.Equal(t, n1, info.GetName())
	require.NotEmpty(t, mnemonic)

	// now, let us delete this key
	err = cstore.Delete(n1)
	require.Nil(t, err, "%+v", err)
	_, err = cstore.Key(n1)
	require.NotNil(t, err)

	// let us re-create it from the mnemonic-phrase
	params := *hd.NewFundraiserParams(0, sdk.CoinType, 0)
	hdPath := params.String()
	newKe, err := cstore.NewAccount(n2, mnemonic, keyring.DefaultBIP39Passphrase, hdPath, algo)
	require.NoError(t, err)
	require.Equal(t, n2, newKe.GetName())
	key, err := info.GetPubKey()
	require.NoError(t, err)
	newKey, err := newKe.GetPubKey()
	require.NoError(t, err)
	require.Equal(t, key.Address(), newKey.Address())
	require.Equal(t, key, newKey)
}

func TestKeyChain_ShouldFailWhenAddingSameGeneratedAccount(t *testing.T) {
	encCfg := simapp.MakeTestEncodingConfig()
	kr, err := keyring.New(t.Name(), keyring.BackendTest, t.TempDir(), nil, encCfg.Marshaler)
	require.NoError(t, err)

	// Given we create a mnemonic
	_, seed, err := kr.NewMnemonic("test", keyring.English, "", keyring.DefaultBIP39Passphrase, hd.Secp256k1)
	require.NoError(t, err)

	require.NoError(t, kr.Delete("test"))

	path := hd.CreateHDPath(118, 0, 0).String()
	_, err = kr.NewAccount("test1", seed, "", path, hd.Secp256k1)
	require.NoError(t, err)

	// Creating another account with different uid but same seed should fail due to have same pub address
	_, err = kr.NewAccount("test2", seed, "", path, hd.Secp256k1)
	require.Error(t, err)
}

func ExampleNew() {
	// Select the encryption and storage for your cryptostore
	encCfg := simapp.MakeTestEncodingConfig()
	cstore := keyring.NewInMemory(encCfg.Marshaler)

	sec := hd.Secp256k1

	// Add keys and see they return in alphabetical order
	bob, _, err := cstore.NewMnemonic("Bob", keyring.English, sdk.FullFundraiserPath, keyring.DefaultBIP39Passphrase, sec)
	if err != nil {
		// this should never happen
		fmt.Println(err)
	} else {
		// return info here just like in List
		fmt.Println(bob.GetName())
	}
	_, _, _ = cstore.NewMnemonic("Alice", keyring.English, sdk.FullFundraiserPath, keyring.DefaultBIP39Passphrase, sec)
	_, _, _ = cstore.NewMnemonic("Carl", keyring.English, sdk.FullFundraiserPath, keyring.DefaultBIP39Passphrase, sec)
	info, _ := cstore.List()
	for _, i := range info {
		fmt.Println(i.GetName())
	}

	// We need to use passphrase to generate a signature
	tx := []byte("deadbeef")
	sig, pub, err := cstore.Sign("Bob", tx)
	if err != nil {
		fmt.Println("don't accept real passphrase")
	}

	// and we can validate the signature with publicly available info
	binfo, _ := cstore.Key("Bob")
	key, _ := binfo.GetPubKey()
	bobKey, _ := bob.GetPubKey()
	if !key.Equals(bobKey) {
		fmt.Println("Get and Create return different keys")
	}

	if pub.Equals(key) {
		fmt.Println("signed by Bob")
	}
	if !pub.VerifySignature(tx, sig) {
		fmt.Println("invalid signature")
	}

	// Output:
	// Bob
	// Alice
	// Bob
	// Carl
	// signed by Bob
}

func TestAltKeyring_List(t *testing.T) {
	dir := t.TempDir()
	encCfg := simapp.MakeTestEncodingConfig()

	kr, err := keyring.New(t.Name(), keyring.BackendTest, dir, nil, encCfg.Marshaler)
	require.NoError(t, err)

	list, err := kr.List()
	require.NoError(t, err)
	require.Empty(t, list)

	// Fails on creating unsupported pubKeyType
	_, _, err = kr.NewMnemonic("failing", keyring.English, sdk.FullFundraiserPath, keyring.DefaultBIP39Passphrase, keyring.NotSupportedAlgo{})
	require.EqualError(t, err, keyring.ErrUnsupportedSigningAlgo.Error())

	// Create 3 keys
	uid1, uid2, uid3 := "Zkey", "Bkey", "Rkey"
	_, _, err = kr.NewMnemonic(uid1, keyring.English, sdk.FullFundraiserPath, keyring.DefaultBIP39Passphrase, hd.Secp256k1)
	require.NoError(t, err)
	_, _, err = kr.NewMnemonic(uid2, keyring.English, sdk.FullFundraiserPath, keyring.DefaultBIP39Passphrase, hd.Secp256k1)
	require.NoError(t, err)
	_, _, err = kr.NewMnemonic(uid3, keyring.English, sdk.FullFundraiserPath, keyring.DefaultBIP39Passphrase, hd.Secp256k1)
	require.NoError(t, err)

	list, err = kr.List()
	require.NoError(t, err)
	require.Len(t, list, 3)

	// Check they are in alphabetical order
	require.Equal(t, uid2, list[0].GetName())
	require.Equal(t, uid3, list[1].GetName())
	require.Equal(t, uid1, list[2].GetName())
}

func TestAltKeyring_NewAccount(t *testing.T) {
	encCfg := simapp.MakeTestEncodingConfig()
	kr, err := keyring.New(t.Name(), keyring.BackendTest, t.TempDir(), nil, encCfg.Marshaler)
	require.NoError(t, err)

	entropy, err := bip39.NewEntropy(keyring.DefaultEntropySize)
	require.NoError(t, err)

	mnemonic, err := bip39.NewMnemonic(entropy)
	require.NoError(t, err)

	uid := "newUid"

	// Fails on creating unsupported pubKeyType
	_, err = kr.NewAccount(uid, mnemonic, keyring.DefaultBIP39Passphrase, sdk.FullFundraiserPath, keyring.NotSupportedAlgo{})
	require.EqualError(t, err, keyring.ErrUnsupportedSigningAlgo.Error())

	ke, err := kr.NewAccount(uid, mnemonic, keyring.DefaultBIP39Passphrase, sdk.FullFundraiserPath, hd.Secp256k1)
	require.NoError(t, err)

	require.Equal(t, uid, ke.GetName())

	list, err := kr.List()
	require.NoError(t, err)
	require.Len(t, list, 1)
}

func TestAltKeyring_Get(t *testing.T) {
	encCfg := simapp.MakeTestEncodingConfig()
	kr, err := keyring.New(t.Name(), keyring.BackendTest, t.TempDir(), nil, encCfg.Marshaler)
	require.NoError(t, err)

	uid := someKey
	mnemonic, _, err := kr.NewMnemonic(uid, keyring.English, sdk.FullFundraiserPath, keyring.DefaultBIP39Passphrase, hd.Secp256k1)
	require.NoError(t, err)

	key, err := kr.Key(uid)
	require.NoError(t, err)
	requireEqualRecord(t, mnemonic, key)
}

func TestAltKeyring_KeyByAddress(t *testing.T) {
	encCfg := simapp.MakeTestEncodingConfig()
	kr, err := keyring.New(t.Name(), keyring.BackendTest, t.TempDir(), nil, encCfg.Marshaler)
	require.NoError(t, err)

	uid := someKey
	mnemonic, _, err := kr.NewMnemonic(uid, keyring.English, sdk.FullFundraiserPath, keyring.DefaultBIP39Passphrase, hd.Secp256k1)
	require.NoError(t, err)

	addr, err := mnemonic.GetAddress()
	require.NoError(t, err)
	key, err := kr.KeyByAddress(addr)
	require.NoError(t, err)
	requireEqualRecord(t, key, mnemonic)
}

func TestAltKeyring_Delete(t *testing.T) {
	encCfg := simapp.MakeTestEncodingConfig()
	kr, err := keyring.New(t.Name(), keyring.BackendTest, t.TempDir(), nil, encCfg.Marshaler)
	require.NoError(t, err)

	uid := someKey
	_, _, err = kr.NewMnemonic(uid, keyring.English, sdk.FullFundraiserPath, keyring.DefaultBIP39Passphrase, hd.Secp256k1)
	require.NoError(t, err)

	list, err := kr.List()
	require.NoError(t, err)
	require.Len(t, list, 1)

	err = kr.Delete(uid)
	require.NoError(t, err)

	list, err = kr.List()
	require.NoError(t, err)
	require.Empty(t, list)
}

func TestAltKeyring_DeleteByAddress(t *testing.T) {
	encCfg := simapp.MakeTestEncodingConfig()
	kr, err := keyring.New(t.Name(), keyring.BackendTest, t.TempDir(), nil, encCfg.Marshaler)
	require.NoError(t, err)

	uid := someKey
	mnemonic, _, err := kr.NewMnemonic(uid, keyring.English, sdk.FullFundraiserPath, keyring.DefaultBIP39Passphrase, hd.Secp256k1)
	require.NoError(t, err)

	list, err := kr.List()
	require.NoError(t, err)
	require.Len(t, list, 1)

	addr, err := mnemonic.GetAddress()
	require.NoError(t, err)
	err = kr.DeleteByAddress(addr)
	require.NoError(t, err)

	list, err = kr.List()
	require.NoError(t, err)
	require.Empty(t, list)
}

func TestAltKeyring_SavePubKey(t *testing.T) {
	encCfg := simapp.MakeTestEncodingConfig()
	kr, err := keyring.New(t.Name(), keyring.BackendTest, t.TempDir(), nil, encCfg.Marshaler)
	require.NoError(t, err)

	list, err := kr.List()
	require.NoError(t, err)
	require.Empty(t, list)

	key := someKey
	priv := ed25519.GenPrivKey()
	pub := priv.PubKey()

	ke, err := kr.SavePubKey(key, pub, hd.Secp256k1.Name())
	require.Nil(t, err)
	key, err := ke.GetPubKey()
	require.NoError(t, err)
	require.Equal(t, pub, key)
	require.Equal(t, key, ke.GetName())
	require.Equal(t, hd.Secp256k1.Name(), ke.GetAlgo())

	list, err = kr.List()
	require.NoError(t, err)
	require.Equal(t, 1, len(list))
}

func TestAltKeyring_SaveMultisig(t *testing.T) {
	encCfg := simapp.MakeTestEncodingConfig()
	kr, err := keyring.New(t.Name(), keyring.BackendTest, t.TempDir(), nil, encCfg.Marshaler)
	require.NoError(t, err)

	mnemonic1, _, err := kr.NewMnemonic("key1", keyring.English, sdk.FullFundraiserPath, keyring.DefaultBIP39Passphrase, hd.Secp256k1)
	require.NoError(t, err)
	mnemonic2, _, err := kr.NewMnemonic("key2", keyring.English, sdk.FullFundraiserPath, keyring.DefaultBIP39Passphrase, hd.Secp256k1)
	require.NoError(t, err)

	key := "multi"
	key1, err := mnemonic1.GetPubKey()
	require.NoError(t, err)
	key2, err := mnemonic2.GetPubKey()
	require.NoError(t, err)
	pub := multisig.NewLegacyAminoPubKey(
		2,
		[]types.PubKey{
			&secp256k1.PubKey{Key: key1.Bytes()},
			&secp256k1.PubKey{Key: key2.Bytes()},
		},
	)

	info, err := kr.SaveMultisig(key, pub)
	require.Nil(t, err)
	infoKey, err := info.GetPubKey()
	require.NoError(t, err)
	require.Equal(t, pub, infoKey)
	require.Equal(t, key, info.GetName())

	list, err := kr.List()
	require.NoError(t, err)
	require.Len(t, list, 3)
}

func TestAltKeyring_Sign(t *testing.T) {
	encCfg := simapp.MakeTestEncodingConfig()
	kr, err := keyring.New(t.Name(), keyring.BackendTest, t.TempDir(), nil, encCfg.Marshaler)
	require.NoError(t, err)

	uid := "jack"
	_, _, err = kr.NewMnemonic(uid, keyring.English, sdk.FullFundraiserPath, keyring.DefaultBIP39Passphrase, hd.Secp256k1)
	require.NoError(t, err)

	msg := []byte("some message")

	sign, key, err := kr.Sign(uid, msg)
	require.NoError(t, err)

	require.True(t, key.VerifySignature(msg, sign))
}

func TestAltKeyring_SignByAddress(t *testing.T) {
	encCfg := simapp.MakeTestEncodingConfig()
	kr, err := keyring.New(t.Name(), keyring.BackendTest, t.TempDir(), nil, encCfg.Marshaler)
	require.NoError(t, err)

	uid := "jack"
	mnemonic, _, err := kr.NewMnemonic(uid, keyring.English, sdk.FullFundraiserPath, keyring.DefaultBIP39Passphrase, hd.Secp256k1)
	require.NoError(t, err)

	msg := []byte("some message")

	addr, err := mnemonic.GetAddress()
	require.NoError(t, err)
	sign, key, err := kr.SignByAddress(addr, msg)
	require.NoError(t, err)

	require.True(t, key.VerifySignature(msg, sign))
}

func TestAltKeyring_ImportExportPrivKey(t *testing.T) {
	encCfg := simapp.MakeTestEncodingConfig()
	kr, err := keyring.New(t.Name(), keyring.BackendTest, t.TempDir(), nil, encCfg.Marshaler)
	require.NoError(t, err)

	uid := theID
	_, _, err = kr.NewMnemonic(uid, keyring.English, sdk.FullFundraiserPath, keyring.DefaultBIP39Passphrase, hd.Secp256k1)
	require.NoError(t, err)

	passphrase := "somePass"
	armor, err := kr.ExportPrivKeyArmor(uid, passphrase)
	require.NoError(t, err)
	err = kr.Delete(uid)
	require.NoError(t, err)
	newUID := otherID
	// Should fail importing with wrong password
	err = kr.ImportPrivKey(newUID, armor, "wrongPass")
	require.EqualError(t, err, "failed to decrypt private key: ciphertext decryption failed")

	err = kr.ImportPrivKey(newUID, armor, passphrase)
	require.NoError(t, err)

	// Should fail importing private key on existing key.
	err = kr.ImportPrivKey(newUID, armor, passphrase)
	require.EqualError(t, err, fmt.Sprintf("cannot overwrite key: %s", newUID))
}

func TestAltKeyring_ImportExportPrivKey_ByAddress(t *testing.T) {
	encCfg := simapp.MakeTestEncodingConfig()
	kr, err := keyring.New(t.Name(), keyring.BackendTest, t.TempDir(), nil, encCfg.Marshaler)
	require.NoError(t, err)

	uid := theID
	mnemonic, _, err := kr.NewMnemonic(uid, keyring.English, sdk.FullFundraiserPath, keyring.DefaultBIP39Passphrase, hd.Secp256k1)
	require.NoError(t, err)

	passphrase := "somePass"
	addr, err := mnemonic.GetAddress()
	require.NoError(t, err)
	armor, err := kr.ExportPrivKeyArmorByAddress(addr, passphrase)
	require.NoError(t, err)
	err = kr.Delete(uid)
	require.NoError(t, err)

	newUID := otherID
	// Should fail importing with wrong password
	err = kr.ImportPrivKey(newUID, armor, "wrongPass")
	require.EqualError(t, err, "failed to decrypt private key: ciphertext decryption failed")

	err = kr.ImportPrivKey(newUID, armor, passphrase)
	require.NoError(t, err)

	// Should fail importing private key on existing key.
	err = kr.ImportPrivKey(newUID, armor, passphrase)
	require.EqualError(t, err, fmt.Sprintf("cannot overwrite key: %s", newUID))
}

func TestAltKeyring_ImportExportPubKey(t *testing.T) {
	encCfg := simapp.MakeTestEncodingConfig()
	kr, err := keyring.New(t.Name(), keyring.BackendTest, t.TempDir(), nil, encCfg.Marshaler)
	require.NoError(t, err)

	uid := theID
	_, _, err = kr.NewMnemonic(uid, keyring.English, sdk.FullFundraiserPath, keyring.DefaultBIP39Passphrase, hd.Secp256k1)
	require.NoError(t, err)

	armor, err := kr.ExportPubKeyArmor(uid)
	require.NoError(t, err)
	err = kr.Delete(uid)
	require.NoError(t, err)

	newUID := otherID
	err = kr.ImportPubKey(newUID, armor)
	require.NoError(t, err)

	// Should fail importing private key on existing key.
	err = kr.ImportPubKey(newUID, armor)
	require.EqualError(t, err, fmt.Sprintf("cannot overwrite key: %s", newUID))
}

func TestAltKeyring_ImportExportPubKey_ByAddress(t *testing.T) {
	encCfg := simapp.MakeTestEncodingConfig()
	kr, err := keyring.New(t.Name(), keyring.BackendTest, t.TempDir(), nil, encCfg.Marshaler)
	require.NoError(t, err)

	uid := theID
	mnemonic, _, err := kr.NewMnemonic(uid, keyring.English, sdk.FullFundraiserPath, keyring.DefaultBIP39Passphrase, hd.Secp256k1)
	require.NoError(t, err)

	addr, err := mnemonic.GetAddress()
	require.NoError(t, err)
	armor, err := kr.ExportPubKeyArmorByAddress(addr)
	require.NoError(t, err)
	err = kr.Delete(uid)
	require.NoError(t, err)

	newUID := otherID
	err = kr.ImportPubKey(newUID, armor)
	require.NoError(t, err)

	// Should fail importing private key on existing key.
	err = kr.ImportPubKey(newUID, armor)
	require.EqualError(t, err, fmt.Sprintf("cannot overwrite key: %s", newUID))
}

func TestAltKeyring_UnsafeExportPrivKeyHex(t *testing.T) {
	encCfg := simapp.MakeTestEncodingConfig()
	kr, err := keyring.New(t.Name(), keyring.BackendTest, t.TempDir(), nil, encCfg.Marshaler)
	require.NoError(t, err)

	uid := theID

	_, _, err = kr.NewMnemonic(uid, keyring.English, sdk.FullFundraiserPath, keyring.DefaultBIP39Passphrase, hd.Secp256k1)
	require.NoError(t, err)

	unsafeKeyring := keyring.NewUnsafe(kr)
	privKey, err := unsafeKeyring.UnsafeExportPrivKeyHex(uid)

	require.NoError(t, err)
	require.Equal(t, 64, len(privKey))

	_, err = hex.DecodeString(privKey)
	require.NoError(t, err)

	// test error on non existing key
	_, err = unsafeKeyring.UnsafeExportPrivKeyHex("non-existing")
	require.Error(t, err)
}

func TestAltKeyring_ConstructorSupportedAlgos(t *testing.T) {
	encCfg := simapp.MakeTestEncodingConfig()
	kr, err := keyring.New(t.Name(), keyring.BackendTest, t.TempDir(), nil, encCfg.Marshaler)
	require.NoError(t, err)

	// should fail when using unsupported signing algorythm.
	_, _, err = kr.NewMnemonic("test", keyring.English, sdk.FullFundraiserPath, keyring.DefaultBIP39Passphrase, keyring.NotSupportedAlgo{})
	require.EqualError(t, err, "unsupported signing algo")

	// but works with default signing algo.
	_, _, err = kr.NewMnemonic("test", keyring.English, sdk.FullFundraiserPath, keyring.DefaultBIP39Passphrase, hd.Secp256k1)
	require.NoError(t, err)

	// but we can create a new keybase with our provided algos.
	kr2, err := keyring.New(t.Name(), keyring.BackendTest, t.TempDir(), nil, encCfg.Marshaler, func(options *keyring.Options) {
		options.SupportedAlgos = keyring.SigningAlgoList{
			keyring.NotSupportedAlgo{},
		}
	})
	require.NoError(t, err)

	// now this new keyring does not fail when signing with provided algo
	_, _, err = kr2.NewMnemonic("test", keyring.English, sdk.FullFundraiserPath, keyring.DefaultBIP39Passphrase, keyring.NotSupportedAlgo{})
	require.NoError(t, err)
}

func TestBackendConfigConstructors(t *testing.T) {
	backend := keyring.NewKWalletBackendKeyringConfig("test", "", nil)
	require.Equal(t, []keyring99designs.BackendType{keyring99designs.KWalletBackend}, backend.AllowedBackends)
	require.Equal(t, "kdewallet", backend.ServiceName)
	require.Equal(t, "test", backend.KWalletAppID)

	backend = keyring.NewPassBackendKeyringConfig("test", "directory", nil)
	require.Equal(t, []keyring99designs.BackendType{keyring99designs.PassBackend}, backend.AllowedBackends)
	require.Equal(t, "test", backend.ServiceName)
	require.Equal(t, "keyring-test", backend.PassPrefix)
}

func requireEqualRecord(t *testing.T, key *keyring.Record, mnemonic *keyring.Record) {
	require.Equal(t, key.GetName(), mnemonic.GetName())

	keyAddr, err := key.GetAddress()
	require.NoError(t, err)
	mnemonicAddr, err := mnemonic.GetAddress()
	require.NoError(t, err)
	require.Equal(t, keyAddr, mnemonicAddr)

	key1, err := key.GetPubKey()
	require.NoError(t, err)
	key2, err := mnemonic.GetPubKey()
	require.NoError(t, err)
	require.Equal(t, key1, key2)
	require.Equal(t, key.GetAlgo(), mnemonic.GetAlgo())
	//	require.Equal(t, key.GetType(), mnemonic.GetType())
}

func accAddr(ke *keyring.Record) (sdk.AccAddress, error) { return ke.GetAddress() }
