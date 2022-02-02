package vrf_test

import (
	"context"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink/core/internal/cltest"
	"github.com/smartcontractkit/chainlink/core/internal/cltest/heavyweight"
	"github.com/smartcontractkit/chainlink/core/internal/testutils/configtest"
	"github.com/smartcontractkit/chainlink/core/internal/testutils/evmtest"
	"github.com/smartcontractkit/chainlink/core/logger"
	"github.com/smartcontractkit/chainlink/core/services/pg"
	"github.com/smartcontractkit/chainlink/core/services/pipeline"
	"github.com/smartcontractkit/chainlink/core/services/pipeline/mocks"
	"github.com/smartcontractkit/chainlink/core/services/vrf"
	"github.com/smartcontractkit/chainlink/core/testdata/testspecs"
	"github.com/smartcontractkit/sqlx"
)

func TestBlockhashDecode(t *testing.T) {

	config, db := heavyweight.FullTestDB(t, "blockhashdecode", true, true)
	key := cltest.MustGenerateRandomKey(t)
	cu := newVRFCoordinatorUniverse(t, key)
	app := cltest.NewApplicationWithConfigAndKeyOnSimulatedBlockchain(t, config, cu.backend, key)
	require.NoError(t, app.Start())

	obsSrc := `
vrf          [type=vrf
             publicKey="$(jobSpec.publicKey)"
             requestBlockHash="$(jobRun.logBlockHash)"
             requestBlockNumber="$(jobRun.logBlockNumber)"
             topics="$(jobRun.logTopics)"]
vrf
`

	vrfSpec := testspecs.GenerateVRFSpec(testspecs.VRFSpecParams{
		ObservationSource: obsSrc,
	})

	jb, err := vrf.ValidatedVRFSpec(vrfSpec.Toml())
	require.NoError(t, err)

	require.NoError(t, app.JobSpawner().CreateJob(&jb))

	require.NotNil(t, jb.Pipeline)
	require.NotNil(t, jb.PipelineSpec)

	blockHash := common.HexToHash("D318579AC61767650C947B4DFE3056C735E49DB8ED704D9FB351817B80D33345")

	vars := pipeline.NewVarsFrom(map[string]interface{}{
		"jobRun": map[string]interface{}{
			"logBlockHash": blockHash[:],
		},
	})

	runner := newRunner(t, db, config)
	t.Log("execute run")
	run, res, err := runner.ExecuteRun(context.Background(), *jb.PipelineSpec, vars, logger.TestLogger(t))
	require.NoError(t, err)
	t.Fatalf("run: %+v\n\nresult: %+v\n", run, res)
}

func newRunner(t *testing.T, db *sqlx.DB, cfg *configtest.TestGeneralConfig) pipeline.Runner {
	cc := evmtest.NewChainSet(t, evmtest.TestChainOpts{DB: db, GeneralConfig: cfg})
	orm := new(mocks.ORM)
	q := pg.NewQ(db, logger.TestLogger(t), cfg)

	orm.On("GetQ").Return(q)
	ethKeyStore := cltest.NewKeyStore(t, db, cfg).Eth()
	r := pipeline.NewRunner(orm, cfg, cc, ethKeyStore, nil, logger.TestLogger(t))
	return r
}
