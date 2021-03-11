package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"io/ioutil"
	"math/big"
	"os"
	"sort"
)

type airDropConfig struct {
	StartBlock    *big.Int
	EndBlock      *big.Int
	AirDropAmount *big.Int
}

type airDropManager struct {
	config     *airDropConfig
	addrList   map[common.Address]*big.Int
	totalSyUSD *big.Int
	rewardList map[common.Address]*big.Float

	client          *ethclient.Client

	amountPreSecond *big.Float
	endTime uint64
}

func readConfigFromJson(file string)*airDropConfig  {
	filePtr, err := os.Open(file)
	if err != nil {
		panic(err)
	}
	defer filePtr.Close()

	c:=new(airDropConfig)

	decoder := json.NewDecoder(filePtr)
	err = decoder.Decode(c)
	if err != nil {
		panic(err)

	}
	return c
}

func NewManager() *airDropManager {
	config:=readConfigFromJson("./config.json")

	gwei:=new(big.Int).SetUint64(1000000000)
	ether:=new(big.Int).Mul(gwei,gwei)
	config.AirDropAmount.Mul(config.AirDropAmount,ether)

	client, err := ethclient.Dial("wss://mainnet.infura.io/ws/v3/5f85acad140a4286858886f080177bc9")
	if err != nil {
		panic(err)
	}

	s:= &airDropManager{
		config:     config,
		addrList:   make(map[common.Address]*big.Int),
		totalSyUSD: new(big.Int),
		rewardList: make(map[common.Address]*big.Float),
		client:     client,
	}
	s.calTokenPerSecond()
	return s
}

func (s *airDropManager)UpdateToken(log types.Log)  {
	from:=common.BytesToAddress(log.Topics[1].Bytes())
	to:=common.BytesToAddress(log.Topics[2].Bytes())
	amount:=new(big.Int).SetBytes(log.Data)
	isMint:=from.String()==common.Address{}.String()

	changedAddr:=from
	if isMint{
		changedAddr=to
	}

	if _,ok:=s.addrList[changedAddr];!ok{
		s.addrList[changedAddr]=new(big.Int)
	}
	if isMint{
		s.addrList[changedAddr].Add(s.addrList[changedAddr],amount)
		s.totalSyUSD.Add(s.totalSyUSD,amount)
	}else{
		s.addrList[changedAddr].Sub(s.addrList[changedAddr],amount)
		s.totalSyUSD.Sub(s.totalSyUSD,amount)
	}
}

func (s *airDropManager)calTokenPerSecond()  {
	fromBlock,err:=s.client.BlockByNumber(context.Background(),s.config.StartBlock)
	if err!=nil{
		panic(err)
	}
	toBlock,err:=s.client.BlockByNumber(context.Background(),s.config.EndBlock)
	if err!=nil{
		panic(err)
	}
	s.endTime=toBlock.Time()
	s.amountPreSecond=new(big.Float).Quo(new(big.Float).SetInt(s.config.AirDropAmount),new(big.Float).SetUint64(toBlock.Time()-fromBlock.Time()))
}

func (s *airDropManager) airDrop(ts uint64)  {
	needToken:=new(big.Float).Mul(new(big.Float).SetUint64(ts),s.amountPreSecond)
	preToken:=new(big.Float).Quo(needToken,new(big.Float).SetInt(s.totalSyUSD))

	for addr,value:=range s.addrList {
		tt:=new(big.Float).Mul(new(big.Float).SetInt(value),preToken)

		if _,ok:=s.rewardList[addr];!ok{
			s.rewardList[addr]=new(big.Float)
		}
		s.rewardList[addr].Add(s.rewardList[addr],tt)
	}
}

type reward struct {
	Addr common.Address
	Value *big.Int
}
type RewardList []reward

func (r RewardList)Len()int  {
	return len(r)
}

func (r RewardList)Less(i,j int)bool  {
	return r[i].Value.Cmp(r[j].Value)>0
}

func (r RewardList)Swap(i,j int)  {
	r[i],r[j]=r[j],r[i]
}

func (s *airDropManager)genRewardFile()  {
	rs:=make(RewardList,0)
	for addr,value:=range s.rewardList{
		r:=reward{
			Addr:  addr,
		}
		r.Value=new(big.Int)
		value.Int(r.Value)
		rs=append(rs,r)
	}
	sort.Sort(rs)

	data,err:=json.MarshalIndent(rs,"","")
	if err!=nil{
		panic(err)
	}
	if err:=ioutil.WriteFile("./reword.json",data,0777);err!=nil{
		panic(err)
	}
}

func (s *airDropManager)display()  {
	nonZeroAmount:=0
	for _,v:=range s.addrList {
		if v.Uint64()!=0{
			nonZeroAmount++
		}
	}


	realReward :=new(big.Float)
	for _,v:=range s.rewardList {
		realReward.Add(realReward,v)
	}

	fmt.Println("空投计算完毕")
	fmt.Printf("   syUSD总数量:%d, 持有syUSD账户数量:%d\n",s.totalSyUSD,nonZeroAmount)
	fmt.Printf("   空投账户数:%d, 预期空投数量:%d, 实际空投总金额:%f\n",len(s.rewardList),s.config.AirDropAmount, realReward)
}

func main()  {
	handler:= NewManager()
	query := ethereum.FilterQuery{
		Addresses: []common.Address{
			common.HexToAddress("0xe5859f4EFc09027A9B718781DCb2C6910CAc6E91"),
		},
		ToBlock:   handler.config.EndBlock,
		Topics: [][]common.Hash{{common.HexToHash("0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef")}},
	}
	logs, err := handler.client.FilterLogs(context.Background(), query)
	if err != nil {
		panic(err)
	}
	fmt.Printf("开始区块:%d, 结束区块:%d\n",handler.config.StartBlock,handler.config.EndBlock)
	fmt.Println("获取Transfer Event结束,数量:",len(logs))

	fmt.Println("获取每个事件对应的时间:开始")
	timeStamps:=make([]uint64,0)
	for index,log:=range logs{
		block,err:=handler.client.HeaderByHash(context.Background(),log.BlockHash)
		if err!=nil{
			panic(err)
		}
		timeStamps=append(timeStamps,block.Time)
		if index%100==0{
			fmt.Printf("     已经获取%d个,还剩%d个\n",index,len(logs)-index)
		}
	}
	fmt.Println("获取每个事件对应的时间:结束")


	lastIndex:=0
	for index,log:=range logs{
		handler.UpdateToken(log)
		if log.BlockNumber>=handler.config.StartBlock.Uint64(){
			if lastIndex==0{
				lastIndex=index
			}
			if timeStamps[index]!=timeStamps[lastIndex] || index==len(logs)-1{
				handler.airDrop(timeStamps[index]-timeStamps[lastIndex])
				lastIndex=index
			}
		}
	}
	handler.airDrop(handler.endTime-timeStamps[len(logs)-1])

	handler.genRewardFile()
	handler.display()
}
