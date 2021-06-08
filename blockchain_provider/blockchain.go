package blockchainprovider

import (
	"bytes"
	"context"
	"crypto/sha256"
	abiPOS "dfile-secondary-node/POS_abi"
	nodeNFT "dfile-secondary-node/node_nft_abi"
	"dfile-secondary-node/shared"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"math/big"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

type StorageInfo struct {
	Nonce        string     `json:"nonce"`
	SignedFsRoot string     `json:"signedFsRoot"`
	Tree         [][][]byte `json:"tree"`
}

const eightKB = 8192

func RegisterNode(password string, ip []string, port int) error {

	nodeAddr, err := shared.DecryptNodeAddr()
	if err != nil {
		return err
	}

	nftAddr := common.HexToAddress("0xBfAfdaE6B77a02A4684D39D1528c873961528342")

	client, err := ethclient.Dial("https://kovan.infura.io/v3/a4a45777ca65485d983c278291e322f2")
	if err != nil {
		return err
	}

	defer client.Close()

	node, err := nodeNFT.NewNodeNft(nftAddr, client)
	if err != nil {
		return err
	}

	ipAddr := [4]uint8{}

	for i, v := range ip {
		intIPPart, err := strconv.Atoi(v)
		if err != nil {
			return err
		}

		ipAddr[i] = uint8(intIPPart)
	}

	opts, _, err := initTrxOpts(client, nodeAddr, password)
	if err != nil {
		return err
	}

	_, err = node.CreateNode(opts, ipAddr, uint16(port))
	if err != nil {
		return err
	}

	return nil
}

func StartMining(password string) {

	nodeAddr, err := shared.DecryptNodeAddr()
	if err != nil {
		log.Fatal(err)
	}

	pathToAccStorage := filepath.Join(shared.AccsDirPath, nodeAddr.String(), shared.StorageDirName)

	regAddr := regexp.MustCompile("^0x[0-9a-fA-F]{40}$")
	regFileName := regexp.MustCompile("[0-9A-Za-z_]")

	client, err := ethclient.Dial("https://kovan.infura.io/v3/a4a45777ca65485d983c278291e322f2")
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	tokenAddress := common.HexToAddress("0x2E8630780A231E8bCf12Ba1172bEB9055deEBF8B")
	instance, err := abiPOS.NewStore(tokenAddress, client)
	if err != nil {
		log.Fatal(err)
	}

	baseDfficulty, err := instance.BaseDifficulty(&bind.CallOpts{})
	if err != nil {
		log.Fatal(err)
	}

	for {

		ctx, _ := context.WithTimeout(context.Background(), time.Minute)

		blockNum, err := client.BlockNumber(ctx)
		if err != nil {
			log.Fatal(err)
		}

		nodeBalance, err := client.BalanceAt(ctx, nodeAddr, big.NewInt(int64(blockNum)))
		if err != nil {
			log.Fatal(err)
		}

		nodeBalanceIsLow := nodeBalance.Cmp(big.NewInt(1500000000000000)) == -1

		if nodeBalanceIsLow {
			fmt.Println("Your account has insufficient funds:", nodeBalance, "wei")
			fmt.Println("Please top up your balance")
			time.Sleep(time.Second * 15)
			continue
		}

		storageProviderAddresses := []string{}

		err = filepath.WalkDir(pathToAccStorage,
			func(path string, info fs.DirEntry, err error) error {
				if err != nil {
					log.Fatal("Fatal error")
				}

				if regAddr.MatchString(info.Name()) {
					storageProviderAddresses = append(storageProviderAddresses, info.Name())
				}

				return nil
			})
		if err != nil {
			log.Fatal("Fatal error")
		}

		if len(storageProviderAddresses) == 0 {
			continue
		}

		for _, spAddress := range storageProviderAddresses {
			storageProviderAddr := common.HexToAddress(spAddress)
			paymentToken, reward, userDifficulty, err := instance.GetUserRewardInfo(&bind.CallOpts{}, storageProviderAddr)
			if err != nil {
				log.Fatal(err)
			}

			fmt.Println(paymentToken, reward, userDifficulty)

			fileNames := []string{}

			pathToStorProviderFiles := filepath.Join(pathToAccStorage, storageProviderAddr.String())

			err = filepath.WalkDir(pathToStorProviderFiles,
				func(path string, info fs.DirEntry, err error) error {
					if err != nil {
						log.Fatal(err)
					}

					if regFileName.MatchString(info.Name()) && len(info.Name()) == 64 {
						fileNames = append(fileNames, info.Name())

					}

					return nil
				})
			if err != nil {
				log.Fatal(err)
			}

			for _, fileName := range fileNames {
				storedFile, err := os.Open(filepath.Join(pathToStorProviderFiles, fileName))
				if err != nil {
					log.Fatal(err)
				}

				storedFileBytes, err := io.ReadAll(storedFile)
				if err != nil {
					log.Fatal(err)
				}

				storedFile.Close()

				blockNum, err := client.BlockNumber(ctx)
				if err != nil {
					log.Fatal(err)
				}

				blockHash, err := instance.GetBlockHash(&bind.CallOpts{}, uint32(blockNum))
				if err != nil {
					log.Fatal(err)
				}

				fileBytesAddrBlockHash := append(storedFileBytes, nodeAddr.Bytes()...)
				fileBytesAddrBlockHash = append(fileBytesAddrBlockHash, blockHash[:]...)

				hashedFileAddrBlock := sha256.Sum256(fileBytesAddrBlockHash)

				hexFileAddrBlock := "0x" + hex.EncodeToString(hashedFileAddrBlock[:])

				decodedBigInt, err := hexutil.DecodeBig(hexFileAddrBlock)
				if err != nil {
					log.Fatal(err)
				}

				remainder := decodedBigInt.Rem(decodedBigInt, baseDfficulty)

				compareResultIsLessUserDifficulty := remainder.CmpAbs(userDifficulty) == -1

				fmt.Println("checked", fileName)

				rewardisEnough := reward.Cmp(big.NewInt(3000000000000000)) == 1

				if compareResultIsLessUserDifficulty && rewardisEnough {
					fmt.Println("Sending Proof for reward", reward)
					err := sendProof(client, password, storedFileBytes, nodeAddr, spAddress)
					if err != nil {
						log.Fatal(err)
					}
				}

			}

		}

		time.Sleep(time.Second * 15)

	}

}

func GetNodeInfo() error {

	nftAddr := common.HexToAddress("0xBfAfdaE6B77a02A4684D39D1528c873961528342")

	client, err := ethclient.Dial("https://kovan.infura.io/v3/a4a45777ca65485d983c278291e322f2")
	if err != nil {
		return err
	}

	defer client.Close()

	node, err := nodeNFT.NewNodeNft(nftAddr, client)
	if err != nil {
		return err
	}

	nodeInfo, err := node.GetNodeById(&bind.CallOpts{}, big.NewInt(2))
	if err != nil {
		return err
	}

	fmt.Println(nodeInfo)

	return nil
}

func initTrxOpts(client *ethclient.Client, nodeAddr common.Address, password string) (*bind.TransactOpts, uint64, error) {
	ctx, _ := context.WithTimeout(context.Background(), time.Minute)

	blockNum, err := client.BlockNumber(ctx)
	if err != nil {
		return nil, 0, err
	}

	transactNonce, err := client.NonceAt(ctx, nodeAddr, big.NewInt(int64(blockNum)))
	if err != nil {
		return nil, 0, err
	}

	chnID, err := client.ChainID(ctx)
	if err != nil {
		return nil, 0, err
	}

	opts := &bind.TransactOpts{
		From:  nodeAddr,
		Nonce: big.NewInt(int64(transactNonce)),
		Signer: func(a common.Address, t *types.Transaction) (*types.Transaction, error) {
			ks := keystore.NewKeyStore(shared.AccsDirPath, keystore.StandardScryptN, keystore.StandardScryptP)
			acs := ks.Accounts()
			for _, ac := range acs {
				if ac.Address == a {
					ks := keystore.NewKeyStore(shared.AccsDirPath, keystore.StandardScryptN, keystore.StandardScryptP)
					ks.TimedUnlock(ac, password, 1)
					return ks.SignTx(ac, t, chnID)
				}
			}
			return t, nil
		},
		Value:    big.NewInt(0),
		GasPrice: big.NewInt(5000000000),
		GasLimit: 1000000,
		Context:  ctx,
		NoSend:   false,
	}

	return opts, blockNum, nil
}

func sendProof(client *ethclient.Client, password string, fileBytes []byte, nodeAddr common.Address, spAddr string) error {

	pathToFsTree := filepath.Join(shared.AccsDirPath, nodeAddr.String(), shared.StorageDirName, spAddr, "tree.json")

	fileFsTree, err := os.Open(pathToFsTree)
	if err != nil {
		return err
	}
	defer fileFsTree.Close()

	treeBytes, err := io.ReadAll(fileFsTree)
	if err != nil {
		return err
	}

	var storageFsStruct StorageInfo

	err = json.Unmarshal(treeBytes, &storageFsStruct)
	if err != nil {
		return err
	}

	eightKBHashes := []string{}

	bytesToProve := fileBytes[:eightKB]

	for i := 0; i < len(fileBytes); i += eightKB {
		hSum := sha256.Sum256(fileBytes[i : i+eightKB])
		eightKBHashes = append(eightKBHashes, hex.EncodeToString(hSum[:]))
	}

	_, fileTree, err := shared.CalcRootHash(eightKBHashes)
	if err != nil {
		return err
	}

	hashFileRoot := fileTree[len(fileTree)-1][0]

	treeToFsRoot := [][][]byte{}

	for _, baseHash := range storageFsStruct.Tree[0] {
		diff := bytes.Compare(hashFileRoot, baseHash)
		if diff == 0 {
			treeToFsRoot = append(treeToFsRoot, fileTree[:len(fileTree)-1]...)
			treeToFsRoot = append(treeToFsRoot, storageFsStruct.Tree...)
		}
	}

	proof := makeProof(fileTree[0][0], treeToFsRoot)

	tokenAddress := common.HexToAddress("0x2E8630780A231E8bCf12Ba1172bEB9055deEBF8B")
	instance, err := abiPOS.NewStore(tokenAddress, client)
	if err != nil {
		return err
	}

	signedFSRootHash, err := hex.DecodeString(storageFsStruct.SignedFsRoot)
	if err != nil {
		return err
	}

	opts, blockNum, err := initTrxOpts(client, nodeAddr, password)
	if err != nil {
		return err
	}

	intNonce, err := strconv.Atoi(storageFsStruct.Nonce)
	if err != nil {
		return err
	}

	_, err = instance.SendProof(opts, common.HexToAddress("0x537F6af3A07e58986Bb5041c304e9Eb2283396CD"), uint32(blockNum), proof[len(proof)-1], uint64(intNonce), signedFSRootHash[:64], bytesToProve, proof)
	if err != nil {
		return err
	}

	fmt.Println("Got some cash 0_o")

	return nil

}

func getPos(hash []byte, list [][]byte) int {
	for i, v := range list {
		diff := bytes.Compare(v, hash)
		if diff == 0 {
			return i
		}
	}

	return -1

}

// Builds merkle tree proof
func makeProof(start []byte, tree [][][]byte) [][32]byte { // returns slice of 32 bytes array because smart contract awaits this type
	stage := 0
	proof := [][32]byte{}

	var firstNodePosition int
	var secondNodePosition int

	for stage < len(tree) {
		pos := getPos(start, tree[stage])
		if pos == -1 {
			break
		}

		if pos%2 != 0 {
			firstNodePosition = pos - 1
			secondNodePosition = pos
		} else {
			firstNodePosition = pos
			secondNodePosition = pos + 1
		}

		if len(tree[stage]) == 1 {
			root := [32]byte{}
			for i, v := range tree[stage][0] {
				root[i] = v
			}

			proof = append(proof, root)

			return proof
		}

		firstNode := [32]byte{}
		for i, v := range tree[stage][firstNodePosition] {
			firstNode[i] = v
		}

		proof = append(proof, firstNode)

		secondNode := [32]byte{}
		for i, v := range tree[stage][secondNodePosition] {
			secondNode[i] = v
		}

		proof = append(proof, secondNode)

		concatBytes := append(tree[stage][firstNodePosition], tree[stage][secondNodePosition]...)
		hSum := sha256.Sum256(concatBytes)

		start = hSum[:]
		stage++

	}

	return proof
}
