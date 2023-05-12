package main

import (
	"fmt"
	"net/http"

	cosmtypes "github.com/cosmos/cosmos-sdk/types"
	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/opts"
	"github.com/go-echarts/go-echarts/v2/types"
)

type Account struct {
	Address     string
	DenomToFund map[string]*cosmtypes.Coin
}

func NewAccount(addr string, denomFunds map[string]*cosmtypes.Coin) *Account {
	return &Account{
		Address:     addr,
		DenomToFund: denomFunds,
	}
}

func (a *Account) Print() {
	s := ""
	for k, v := range a.DenomToFund {
		s += fmt.Sprintf("%s:%s ", k, v)
	}
	fmt.Printf("Account: %s, funds=%s\n", a.Address, s)
}

// constant product formula
type Pool struct {
	BaseDecimal cosmtypes.Int
	BaseFund    cosmtypes.Coin

	QuoteDecimal cosmtypes.Int
	QuoteFund    cosmtypes.Coin
	Constant     cosmtypes.Int
}

func NewPool(baseFund, quoteFund cosmtypes.Coin, baseDecimal, quoteDecimal int, k cosmtypes.Int) *Pool {
	bd := cosmtypes.OneInt()
	qd := cosmtypes.OneInt()
	for i := 0; i < baseDecimal; i++ {
		bd = bd.MulRaw(10)
	}

	for i := 0; i < quoteDecimal; i++ {
		qd = qd.MulRaw(10)
	}

	return &Pool{
		BaseFund:     baseFund,
		BaseDecimal:  bd,
		QuoteFund:    quoteFund,
		QuoteDecimal: qd,
		Constant:     k,
	}
}

// non-profit deposit
func (p *Pool) Deposit(baseFunds, quoteFund cosmtypes.Coin) (refund []cosmtypes.Coin) {
	currentRatio := cosmtypes.NewDecFromInt(p.BaseFund.Amount).Quo(cosmtypes.NewDecFromInt(p.QuoteFund.Amount))
	// only get enough funds with current ratio
	// after deposit, currentRatio must stay the same
	expectedQuote := cosmtypes.NewDecFromInt(baseFunds.Amount).Quo(currentRatio).TruncateInt()

	if expectedQuote.LTE(quoteFund.Amount) {
		// refunds quote funds
		refundQuote := quoteFund.Amount.Sub(expectedQuote)
		p.BaseFund = p.BaseFund.Add(baseFunds)
		p.QuoteFund.Amount = p.QuoteFund.Amount.Add(expectedQuote)

		return []cosmtypes.Coin{
			{
				Amount: refundQuote,
				Denom:  p.QuoteFund.Denom,
			},
		}
	}

	// else deposit all quote and refund base
	expectedBase := currentRatio.Mul(cosmtypes.NewDecFromInt(quoteFund.Amount)).TruncateInt()
	refundBase := baseFunds.Amount.Sub(expectedBase)
	p.QuoteFund = p.QuoteFund.Add(quoteFund)
	p.BaseFund.Amount = p.BaseFund.Amount.Add(baseFunds.Amount)
	return []cosmtypes.Coin{
		{
			Amount: refundBase,
			Denom:  p.BaseFund.Denom,
		},
	}
}

func (p *Pool) Buy(bidFund cosmtypes.Coin, account *Account) {
	if bidFund.Denom != p.QuoteFund.Denom {
		panic(fmt.Errorf("invalid denom to bid for this pool: %s", bidFund.Denom))
	}

	if bidFund.Amount.GT(account.DenomToFund[bidFund.Denom].Amount) {
		panic(fmt.Errorf("insufficent fund: bid fund must not be greater than account fund"))
	}

	tmpQuote := p.QuoteFund.Add(bidFund)
	expectedBase := cosmtypes.NewDecFromInt(p.Constant).Quo(cosmtypes.NewDecFromInt(tmpQuote.Amount)).TruncateInt()
	sentBase := p.BaseFund.Amount.Sub(expectedBase)
	account.DenomToFund[p.BaseFund.Denom].Amount = account.DenomToFund[p.BaseFund.Denom].Amount.Add(
		sentBase,
	)
	account.DenomToFund[p.QuoteFund.Denom].Amount = account.DenomToFund[p.QuoteFund.Denom].Amount.Sub(
		bidFund.Amount,
	)

	p.BaseFund.Amount = expectedBase
	p.QuoteFund = tmpQuote
}

func (p *Pool) Price() cosmtypes.Dec {
	return cosmtypes.NewDecFromInt(p.QuoteFund.Amount).Mul(cosmtypes.NewDecFromInt(p.BaseDecimal)).Quo(
		cosmtypes.NewDecFromInt(p.QuoteDecimal)).Quo(
		cosmtypes.NewDecFromInt(p.BaseFund.Amount),
	)
}

func (p *Pool) Sell(askFund cosmtypes.Coin, account *Account) {
	if askFund.Denom != p.QuoteFund.Denom {
		panic(fmt.Errorf("invalid denom to bid for this pool: %s", askFund.Denom))
	}

	if askFund.Amount.GT(account.DenomToFund[askFund.Denom].Amount) {
		panic(fmt.Errorf("insufficent fund: ask fund must not be greater than account fund"))
	}

	tmpBase := p.QuoteFund.Add(askFund)
	expectedQuote := cosmtypes.NewDecFromInt(p.Constant).Quo(cosmtypes.NewDecFromInt(tmpBase.Amount)).TruncateInt()
	sentQuote := p.QuoteFund.Amount.Sub(expectedQuote)
	account.DenomToFund[p.QuoteFund.Denom].Amount = account.DenomToFund[p.QuoteFund.Denom].Amount.Add(
		sentQuote,
	)
	account.DenomToFund[p.BaseFund.Denom].Amount = account.DenomToFund[p.BaseFund.Denom].Amount.Sub(
		askFund.Amount,
	)

	p.BaseFund = tmpBase
	p.QuoteFund.Amount = expectedQuote
}

func (p *Pool) Print() {
	fmt.Printf("Pool: asset: %s:%s %s:%s, price($):%s\n", p.BaseFund.Denom, p.BaseFund.Amount, p.QuoteFund.Denom, p.QuoteFund.Amount, p.Price().String())
}

func paintHandler(X []float64, Y []float64) func(w http.ResponseWriter, _ *http.Request) {
	return func(w http.ResponseWriter, _ *http.Request) {
		// create a new line instance
		line := charts.NewLine()
		// set some global options like Title/Legend/ToolTip or anything else
		line.SetGlobalOptions(
			charts.WithInitializationOpts(opts.Initialization{Theme: types.ThemeWesteros}),
			charts.WithTitleOpts(opts.Title{
				Title: "Pool asset",
			}))

		var xline = []opts.LineData{}
		for _, y := range X {
			xline = append(xline, opts.LineData{Value: y})
		}
		line.SetXAxis(Y).AddSeries("Asset", xline)
		line.Render(w)
	}
}

func main() {
	denomINJ := "inj"
	denomUSDT := "usdt"
	accountA := NewAccount("0x1", map[string]*cosmtypes.Coin{
		denomINJ: {
			Denom:  denomINJ,
			Amount: cosmtypes.MustNewDecFromStr("10000000000000000000").TruncateInt(),
		},
		denomUSDT: {
			Denom:  denomUSDT,
			Amount: cosmtypes.MustNewDecFromStr("1000000000").TruncateInt(),
		},
	})

	pool := NewPool(
		cosmtypes.NewCoin(denomINJ, cosmtypes.MustNewDecFromStr("100000000000000000000").TruncateInt()),
		cosmtypes.NewCoin(denomUSDT, cosmtypes.MustNewDecFromStr("800000000").TruncateInt()),
		18,
		6,
		cosmtypes.MustNewDecFromStr("80000000000000000000000000000").TruncateInt(),
	)
	accountA.Print()
	pool.Print()

	assetINJ := []float64{}
	assetUSDT := []float64{}
	injAmount, usdtAmount := cosmtypes.NewDecFromInt(pool.BaseFund.Amount).Quo(cosmtypes.NewDecFromInt(pool.BaseDecimal)), cosmtypes.NewDecFromInt(pool.QuoteFund.Amount).Quo(cosmtypes.NewDecFromInt(pool.QuoteDecimal))
	assetINJ = append(assetINJ, injAmount.MustFloat64())
	assetUSDT = append(assetUSDT, usdtAmount.MustFloat64())
	for i := 1; i <= 1000; i++ {
		fmt.Printf("buy %d-th\n", i)
		amount := cosmtypes.NewCoin(pool.QuoteFund.Denom, cosmtypes.MustNewDecFromStr("1000000").TruncateInt())
		pool.Buy(amount, accountA)
		accountA.Print()
		pool.Print()
		injAmount, usdtAmount := cosmtypes.NewDecFromInt(pool.BaseFund.Amount).Quo(cosmtypes.NewDecFromInt(pool.BaseDecimal)), cosmtypes.NewDecFromInt(pool.QuoteFund.Amount).Quo(cosmtypes.NewDecFromInt(pool.QuoteDecimal))
		assetINJ = append(assetINJ, injAmount.MustFloat64())
		assetUSDT = append(assetUSDT, usdtAmount.MustFloat64())
		fmt.Println("-------")
	}

	fmt.Println("graph is painted here: http://localhost:8081")
	http.HandleFunc("/", paintHandler(assetINJ, assetUSDT))
	http.ListenAndServe(":8081", nil)
}
