package capi

import (
	"unsafe"

	portout "github.com/bnema/purego-sqlite/internal/ports/out"
)

// Compile-time check that Bridge implements portout.CAPI.
var _ portout.CAPI = (*Bridge)(nil)

// Bridge is a stateless adapter that implements portout.CAPI by delegating
// every method to the corresponding package-level function variable.
type Bridge struct{}

// NewBridge returns a new Bridge instance.
func NewBridge() *Bridge {
	return &Bridge{}
}

// --- Lifecycle ---

func (b *Bridge) Sqlite3Close(arg0 unsafe.Pointer) int32 {
	return Sqlite3Close(arg0)
}

func (b *Bridge) Sqlite3CloseV2(arg0 unsafe.Pointer) int32 {
	return Sqlite3CloseV2(arg0)
}

func (b *Bridge) Sqlite3Initialize() int32 {
	return Sqlite3Initialize()
}

func (b *Bridge) Sqlite3Shutdown() int32 {
	return Sqlite3Shutdown()
}

func (b *Bridge) Sqlite3OsInit() int32 {
	return Sqlite3OsInit()
}

func (b *Bridge) Sqlite3OsEnd() int32 {
	return Sqlite3OsEnd()
}

func (b *Bridge) Sqlite3Open(filename string, ppDb unsafe.Pointer) int32 {
	return Sqlite3Open(filename, ppDb)
}

func (b *Bridge) Sqlite3Open16(filename unsafe.Pointer, ppDb unsafe.Pointer) int32 {
	return Sqlite3Open16(filename, ppDb)
}

func (b *Bridge) Sqlite3OpenV2(filename string, ppDb unsafe.Pointer, flags int32, zVfs string) int32 {
	return Sqlite3OpenV2(filename, ppDb, flags, zVfs)
}

// --- Prepare ---

func (b *Bridge) Sqlite3Prepare(dB unsafe.Pointer, zSql string, nByte int32, ppStmt unsafe.Pointer, pzTail unsafe.Pointer) int32 {
	return Sqlite3Prepare(dB, zSql, nByte, ppStmt, pzTail)
}

func (b *Bridge) Sqlite3PrepareV2(dB unsafe.Pointer, zSql string, nByte int32, ppStmt unsafe.Pointer, pzTail unsafe.Pointer) int32 {
	return Sqlite3PrepareV2(dB, zSql, nByte, ppStmt, pzTail)
}

func (b *Bridge) Sqlite3PrepareV3(dB unsafe.Pointer, zSql string, nByte int32, prepFlags uint32, ppStmt unsafe.Pointer, pzTail unsafe.Pointer) int32 {
	return Sqlite3PrepareV3(dB, zSql, nByte, prepFlags, ppStmt, pzTail)
}

func (b *Bridge) Sqlite3Prepare16(dB unsafe.Pointer, zSql unsafe.Pointer, nByte int32, ppStmt unsafe.Pointer, pzTail unsafe.Pointer) int32 {
	return Sqlite3Prepare16(dB, zSql, nByte, ppStmt, pzTail)
}

func (b *Bridge) Sqlite3Prepare16V2(dB unsafe.Pointer, zSql unsafe.Pointer, nByte int32, ppStmt unsafe.Pointer, pzTail unsafe.Pointer) int32 {
	return Sqlite3Prepare16V2(dB, zSql, nByte, ppStmt, pzTail)
}

func (b *Bridge) Sqlite3Prepare16V3(dB unsafe.Pointer, zSql unsafe.Pointer, nByte int32, prepFlags uint32, ppStmt unsafe.Pointer, pzTail unsafe.Pointer) int32 {
	return Sqlite3Prepare16V3(dB, zSql, nByte, prepFlags, ppStmt, pzTail)
}

func (b *Bridge) Sqlite3SQL(pStmt unsafe.Pointer) string {
	return Sqlite3SQL(pStmt)
}

func (b *Bridge) Sqlite3ExpandedSQL(pStmt unsafe.Pointer) string {
	return Sqlite3ExpandedSQL(pStmt)
}

func (b *Bridge) Sqlite3NormalizedSQL(pStmt unsafe.Pointer) string {
	return Sqlite3NormalizedSQL(pStmt)
}

func (b *Bridge) Sqlite3StmtReadonly(pStmt unsafe.Pointer) int32 {
	return Sqlite3StmtReadonly(pStmt)
}

func (b *Bridge) Sqlite3StmtIsexplain(pStmt unsafe.Pointer) int32 {
	return Sqlite3StmtIsexplain(pStmt)
}

func (b *Bridge) Sqlite3StmtExplain(pStmt unsafe.Pointer, eMode int32) int32 {
	return Sqlite3StmtExplain(pStmt, eMode)
}

func (b *Bridge) Sqlite3StmtBusy(arg0 unsafe.Pointer) int32 {
	return Sqlite3StmtBusy(arg0)
}

func (b *Bridge) Sqlite3Step(arg0 unsafe.Pointer) int32 {
	return Sqlite3Step(arg0)
}

func (b *Bridge) Sqlite3Finalize(pStmt unsafe.Pointer) int32 {
	return Sqlite3Finalize(pStmt)
}

func (b *Bridge) Sqlite3Reset(pStmt unsafe.Pointer) int32 {
	return Sqlite3Reset(pStmt)
}

func (b *Bridge) Sqlite3ResetAutoExtension() {
	Sqlite3ResetAutoExtension()
}

func (b *Bridge) Sqlite3StmtStatus(arg0 unsafe.Pointer, op int32, resetFlg int32) int32 {
	return Sqlite3StmtStatus(arg0, op, resetFlg)
}

func (b *Bridge) Sqlite3StmtScanstatus(pStmt unsafe.Pointer, idx int32, iScanStatusOp int32, pOut unsafe.Pointer) int32 {
	return Sqlite3StmtScanstatus(pStmt, idx, iScanStatusOp, pOut)
}

func (b *Bridge) Sqlite3StmtScanstatusV2(pStmt unsafe.Pointer, idx int32, iScanStatusOp int32, flags int32, pOut unsafe.Pointer) int32 {
	return Sqlite3StmtScanstatusV2(pStmt, idx, iScanStatusOp, flags, pOut)
}

func (b *Bridge) Sqlite3StmtScanstatusReset(arg0 unsafe.Pointer) {
	Sqlite3StmtScanstatusReset(arg0)
}

// --- Column ---

func (b *Bridge) Sqlite3ColumnCount(pStmt unsafe.Pointer) int32 {
	return Sqlite3ColumnCount(pStmt)
}

func (b *Bridge) Sqlite3ColumnName(arg0 unsafe.Pointer, n int32) string {
	return Sqlite3ColumnName(arg0, n)
}

func (b *Bridge) Sqlite3ColumnName16(arg0 unsafe.Pointer, n int32) unsafe.Pointer {
	return Sqlite3ColumnName16(arg0, n)
}

func (b *Bridge) Sqlite3ColumnDatabaseName(arg0 unsafe.Pointer, arg1 uintptr) string {
	return Sqlite3ColumnDatabaseName(arg0, arg1)
}

func (b *Bridge) Sqlite3ColumnDatabaseName16(arg0 unsafe.Pointer, arg1 uintptr) unsafe.Pointer {
	return Sqlite3ColumnDatabaseName16(arg0, arg1)
}

func (b *Bridge) Sqlite3ColumnTableName(arg0 unsafe.Pointer, arg1 uintptr) string {
	return Sqlite3ColumnTableName(arg0, arg1)
}

func (b *Bridge) Sqlite3ColumnTableName16(arg0 unsafe.Pointer, arg1 uintptr) unsafe.Pointer {
	return Sqlite3ColumnTableName16(arg0, arg1)
}

func (b *Bridge) Sqlite3ColumnOriginName(arg0 unsafe.Pointer, arg1 uintptr) string {
	return Sqlite3ColumnOriginName(arg0, arg1)
}

func (b *Bridge) Sqlite3ColumnOriginName16(arg0 unsafe.Pointer, arg1 uintptr) unsafe.Pointer {
	return Sqlite3ColumnOriginName16(arg0, arg1)
}

func (b *Bridge) Sqlite3ColumnDecltype(arg0 unsafe.Pointer, arg1 uintptr) string {
	return Sqlite3ColumnDecltype(arg0, arg1)
}

func (b *Bridge) Sqlite3ColumnDecltype16(arg0 unsafe.Pointer, arg1 uintptr) unsafe.Pointer {
	return Sqlite3ColumnDecltype16(arg0, arg1)
}

func (b *Bridge) Sqlite3DataCount(pStmt unsafe.Pointer) int32 {
	return Sqlite3DataCount(pStmt)
}

func (b *Bridge) Sqlite3ColumnBlob(arg0 unsafe.Pointer, iCol int32) unsafe.Pointer {
	return Sqlite3ColumnBlob(arg0, iCol)
}

func (b *Bridge) Sqlite3ColumnDouble(arg0 unsafe.Pointer, iCol int32) float64 {
	return Sqlite3ColumnDouble(arg0, iCol)
}

func (b *Bridge) Sqlite3ColumnInt(arg0 unsafe.Pointer, iCol int32) int32 {
	return Sqlite3ColumnInt(arg0, iCol)
}

func (b *Bridge) Sqlite3ColumnInt64(arg0 unsafe.Pointer, iCol int32) int64 {
	return Sqlite3ColumnInt64(arg0, iCol)
}

func (b *Bridge) Sqlite3ColumnText(arg0 unsafe.Pointer, iCol int32) *byte {
	return Sqlite3ColumnText(arg0, iCol)
}

func (b *Bridge) Sqlite3ColumnText16(arg0 unsafe.Pointer, iCol int32) unsafe.Pointer {
	return Sqlite3ColumnText16(arg0, iCol)
}

func (b *Bridge) Sqlite3ColumnValue(arg0 unsafe.Pointer, iCol int32) unsafe.Pointer {
	return Sqlite3ColumnValue(arg0, iCol)
}

func (b *Bridge) Sqlite3ColumnBytes(arg0 unsafe.Pointer, iCol int32) int32 {
	return Sqlite3ColumnBytes(arg0, iCol)
}

func (b *Bridge) Sqlite3ColumnBytes16(arg0 unsafe.Pointer, iCol int32) int32 {
	return Sqlite3ColumnBytes16(arg0, iCol)
}

func (b *Bridge) Sqlite3ColumnType(arg0 unsafe.Pointer, iCol int32) int32 {
	return Sqlite3ColumnType(arg0, iCol)
}

// --- Bind ---

func (b *Bridge) Sqlite3BindBlob(arg0 unsafe.Pointer, arg1 uintptr, arg2 unsafe.Pointer, n int32, arg3 unsafe.Pointer) int32 {
	return Sqlite3BindBlob(arg0, arg1, arg2, n, arg3)
}

func (b *Bridge) Sqlite3BindBlob64(arg0 unsafe.Pointer, arg1 uintptr, arg2 unsafe.Pointer, sqlite3Uint64 uintptr, arg3 unsafe.Pointer) int32 {
	return Sqlite3BindBlob64(arg0, arg1, arg2, sqlite3Uint64, arg3)
}

func (b *Bridge) Sqlite3BindDouble(arg0 unsafe.Pointer, arg1 uintptr, arg2 float64) int32 {
	return Sqlite3BindDouble(arg0, arg1, arg2)
}

func (b *Bridge) Sqlite3BindInt(arg0 unsafe.Pointer, arg1 uintptr, arg2 uintptr) int32 {
	return Sqlite3BindInt(arg0, arg1, arg2)
}

func (b *Bridge) Sqlite3BindInt64(arg0 unsafe.Pointer, arg1 uintptr, sqlite3Int64 uintptr) int32 {
	return Sqlite3BindInt64(arg0, arg1, sqlite3Int64)
}

func (b *Bridge) Sqlite3BindNull(arg0 unsafe.Pointer, arg1 uintptr) int32 {
	return Sqlite3BindNull(arg0, arg1)
}

func (b *Bridge) Sqlite3BindText(arg0 unsafe.Pointer, arg1 uintptr, arg2 string, arg3 uintptr, arg4 unsafe.Pointer) int32 {
	return Sqlite3BindText(arg0, arg1, arg2, arg3, arg4)
}

func (b *Bridge) Sqlite3BindText16(arg0 unsafe.Pointer, arg1 uintptr, arg2 unsafe.Pointer, arg3 uintptr, arg4 unsafe.Pointer) int32 {
	return Sqlite3BindText16(arg0, arg1, arg2, arg3, arg4)
}

func (b *Bridge) Sqlite3BindText64(arg0 unsafe.Pointer, arg1 uintptr, arg2 string, sqlite3Uint64 uintptr, arg3 unsafe.Pointer, encoding uintptr) int32 {
	return Sqlite3BindText64(arg0, arg1, arg2, sqlite3Uint64, arg3, encoding)
}

func (b *Bridge) Sqlite3BindValue(arg0 unsafe.Pointer, arg1 uintptr, arg2 unsafe.Pointer) int32 {
	return Sqlite3BindValue(arg0, arg1, arg2)
}

func (b *Bridge) Sqlite3BindPointer(arg0 unsafe.Pointer, arg1 uintptr, arg2 unsafe.Pointer, arg3 string, arg4 unsafe.Pointer) int32 {
	return Sqlite3BindPointer(arg0, arg1, arg2, arg3, arg4)
}

func (b *Bridge) Sqlite3BindZeroblob(arg0 unsafe.Pointer, arg1 uintptr, n int32) int32 {
	return Sqlite3BindZeroblob(arg0, arg1, n)
}

func (b *Bridge) Sqlite3BindZeroblob64(arg0 unsafe.Pointer, arg1 uintptr, sqlite3Uint64 uintptr) int32 {
	return Sqlite3BindZeroblob64(arg0, arg1, sqlite3Uint64)
}

func (b *Bridge) Sqlite3BindParameterCount(arg0 unsafe.Pointer) int32 {
	return Sqlite3BindParameterCount(arg0)
}

func (b *Bridge) Sqlite3BindParameterName(arg0 unsafe.Pointer, arg1 uintptr) string {
	return Sqlite3BindParameterName(arg0, arg1)
}

func (b *Bridge) Sqlite3BindParameterIndex(arg0 unsafe.Pointer, zName string) int32 {
	return Sqlite3BindParameterIndex(arg0, zName)
}

func (b *Bridge) Sqlite3ClearBindings(arg0 unsafe.Pointer) int32 {
	return Sqlite3ClearBindings(arg0)
}

// --- Metadata ---

func (b *Bridge) Sqlite3LastInsertRowid(arg0 unsafe.Pointer) int64 {
	return Sqlite3LastInsertRowid(arg0)
}

func (b *Bridge) Sqlite3SetLastInsertRowid(arg0 unsafe.Pointer, sqlite3Int64 uintptr) {
	Sqlite3SetLastInsertRowid(arg0, sqlite3Int64)
}

func (b *Bridge) Sqlite3Changes(arg0 unsafe.Pointer) int32 {
	return Sqlite3Changes(arg0)
}

func (b *Bridge) Sqlite3Changes64(arg0 unsafe.Pointer) int64 {
	return Sqlite3Changes64(arg0)
}

func (b *Bridge) Sqlite3TotalChanges(arg0 unsafe.Pointer) int32 {
	return Sqlite3TotalChanges(arg0)
}

func (b *Bridge) Sqlite3TotalChanges64(arg0 unsafe.Pointer) int64 {
	return Sqlite3TotalChanges64(arg0)
}

func (b *Bridge) Sqlite3Errcode(dB unsafe.Pointer) int32 {
	return Sqlite3Errcode(dB)
}

func (b *Bridge) Sqlite3ExtendedErrcode(dB unsafe.Pointer) int32 {
	return Sqlite3ExtendedErrcode(dB)
}

func (b *Bridge) Sqlite3Errmsg(arg0 unsafe.Pointer) string {
	return Sqlite3Errmsg(arg0)
}

func (b *Bridge) Sqlite3Errmsg16(arg0 unsafe.Pointer) unsafe.Pointer {
	return Sqlite3Errmsg16(arg0)
}

func (b *Bridge) Sqlite3Errstr(arg0 uintptr) string {
	return Sqlite3Errstr(arg0)
}

func (b *Bridge) Sqlite3ErrorOffset(dB unsafe.Pointer) int32 {
	return Sqlite3ErrorOffset(dB)
}

// --- Exec ---

func (b *Bridge) Sqlite3Exec(arg0 unsafe.Pointer, sQL string, arg1 unsafe.Pointer, arg2 unsafe.Pointer, errmsg unsafe.Pointer) int32 {
	return Sqlite3Exec(arg0, sQL, arg1, arg2, errmsg)
}

func (b *Bridge) Sqlite3Interrupt(arg0 unsafe.Pointer) {
	Sqlite3Interrupt(arg0)
}

func (b *Bridge) Sqlite3IsInterrupted(arg0 unsafe.Pointer) int32 {
	return Sqlite3IsInterrupted(arg0)
}

// --- Backup ---

func (b *Bridge) Sqlite3BackupInit(pDest unsafe.Pointer, zDestName string, pSource unsafe.Pointer, zSourceName string) unsafe.Pointer {
	return Sqlite3BackupInit(pDest, zDestName, pSource, zSourceName)
}

func (b *Bridge) Sqlite3BackupStep(p unsafe.Pointer, nPage int32) int32 {
	return Sqlite3BackupStep(p, nPage)
}

func (b *Bridge) Sqlite3BackupFinish(p unsafe.Pointer) int32 {
	return Sqlite3BackupFinish(p)
}

func (b *Bridge) Sqlite3BackupRemaining(p unsafe.Pointer) int32 {
	return Sqlite3BackupRemaining(p)
}

func (b *Bridge) Sqlite3BackupPagecount(p unsafe.Pointer) int32 {
	return Sqlite3BackupPagecount(p)
}

// --- Blob ---

func (b *Bridge) Sqlite3BlobOpen(arg0 unsafe.Pointer, zDb string, zTable string, zColumn string, iRow int64, flags int32, ppBlob unsafe.Pointer) int32 {
	return Sqlite3BlobOpen(arg0, zDb, zTable, zColumn, iRow, flags, ppBlob)
}

func (b *Bridge) Sqlite3BlobReopen(arg0 unsafe.Pointer, sqlite3Int64 uintptr) int32 {
	return Sqlite3BlobReopen(arg0, sqlite3Int64)
}

func (b *Bridge) Sqlite3BlobClose(arg0 unsafe.Pointer) int32 {
	return Sqlite3BlobClose(arg0)
}

func (b *Bridge) Sqlite3BlobBytes(arg0 unsafe.Pointer) int32 {
	return Sqlite3BlobBytes(arg0)
}

func (b *Bridge) Sqlite3BlobRead(arg0 unsafe.Pointer, z unsafe.Pointer, n int32, iOffset int32) int32 {
	return Sqlite3BlobRead(arg0, z, n, iOffset)
}

func (b *Bridge) Sqlite3BlobWrite(arg0 unsafe.Pointer, z unsafe.Pointer, n int32, iOffset int32) int32 {
	return Sqlite3BlobWrite(arg0, z, n, iOffset)
}

// --- Context ---

func (b *Bridge) Sqlite3AggregateContext(arg0 unsafe.Pointer, nBytes int32) unsafe.Pointer {
	return Sqlite3AggregateContext(arg0, nBytes)
}

func (b *Bridge) Sqlite3ContextDBHandle(arg0 unsafe.Pointer) unsafe.Pointer {
	return Sqlite3ContextDBHandle(arg0)
}

// --- Value ---

func (b *Bridge) Sqlite3ValueBlob(arg0 unsafe.Pointer) unsafe.Pointer {
	return Sqlite3ValueBlob(arg0)
}

func (b *Bridge) Sqlite3ValueDouble(arg0 unsafe.Pointer) float64 {
	return Sqlite3ValueDouble(arg0)
}

func (b *Bridge) Sqlite3ValueInt(arg0 unsafe.Pointer) int32 {
	return Sqlite3ValueInt(arg0)
}

func (b *Bridge) Sqlite3ValueInt64(arg0 unsafe.Pointer) int64 {
	return Sqlite3ValueInt64(arg0)
}

func (b *Bridge) Sqlite3ValuePointer(arg0 unsafe.Pointer, arg1 string) unsafe.Pointer {
	return Sqlite3ValuePointer(arg0, arg1)
}

func (b *Bridge) Sqlite3ValueText(arg0 unsafe.Pointer) *byte {
	return Sqlite3ValueText(arg0)
}

func (b *Bridge) Sqlite3ValueText16(arg0 unsafe.Pointer) unsafe.Pointer {
	return Sqlite3ValueText16(arg0)
}

func (b *Bridge) Sqlite3ValueText16le(arg0 unsafe.Pointer) unsafe.Pointer {
	return Sqlite3ValueText16le(arg0)
}

func (b *Bridge) Sqlite3ValueText16be(arg0 unsafe.Pointer) unsafe.Pointer {
	return Sqlite3ValueText16be(arg0)
}

func (b *Bridge) Sqlite3ValueBytes(arg0 unsafe.Pointer) int32 {
	return Sqlite3ValueBytes(arg0)
}

func (b *Bridge) Sqlite3ValueBytes16(arg0 unsafe.Pointer) int32 {
	return Sqlite3ValueBytes16(arg0)
}

func (b *Bridge) Sqlite3ValueType(arg0 unsafe.Pointer) int32 {
	return Sqlite3ValueType(arg0)
}

func (b *Bridge) Sqlite3ValueNumericType(arg0 unsafe.Pointer) int32 {
	return Sqlite3ValueNumericType(arg0)
}

func (b *Bridge) Sqlite3ValueNochange(arg0 unsafe.Pointer) int32 {
	return Sqlite3ValueNochange(arg0)
}

func (b *Bridge) Sqlite3ValueFrombind(arg0 unsafe.Pointer) int32 {
	return Sqlite3ValueFrombind(arg0)
}

func (b *Bridge) Sqlite3ValueEncoding(arg0 unsafe.Pointer) int32 {
	return Sqlite3ValueEncoding(arg0)
}

func (b *Bridge) Sqlite3ValueSubtype(arg0 unsafe.Pointer) uint32 {
	return Sqlite3ValueSubtype(arg0)
}

func (b *Bridge) Sqlite3ValueDup(arg0 unsafe.Pointer) unsafe.Pointer {
	return Sqlite3ValueDup(arg0)
}

func (b *Bridge) Sqlite3ValueFree(arg0 unsafe.Pointer) {
	Sqlite3ValueFree(arg0)
}

// --- Result ---

func (b *Bridge) Sqlite3ResultBlob(arg0 unsafe.Pointer, arg1 unsafe.Pointer, arg2 uintptr, arg3 unsafe.Pointer) {
	Sqlite3ResultBlob(arg0, arg1, arg2, arg3)
}

func (b *Bridge) Sqlite3ResultBlob64(arg0 unsafe.Pointer, arg1 unsafe.Pointer, sqlite3Uint64 uintptr, arg2 unsafe.Pointer) {
	Sqlite3ResultBlob64(arg0, arg1, sqlite3Uint64, arg2)
}

func (b *Bridge) Sqlite3ResultDouble(arg0 unsafe.Pointer, arg1 uintptr) {
	Sqlite3ResultDouble(arg0, arg1)
}

func (b *Bridge) Sqlite3ResultError(arg0 unsafe.Pointer, arg1 string, arg2 uintptr) {
	Sqlite3ResultError(arg0, arg1, arg2)
}

func (b *Bridge) Sqlite3ResultError16(arg0 unsafe.Pointer, arg1 unsafe.Pointer, arg2 uintptr) {
	Sqlite3ResultError16(arg0, arg1, arg2)
}

func (b *Bridge) Sqlite3ResultErrorToobig(arg0 unsafe.Pointer) {
	Sqlite3ResultErrorToobig(arg0)
}

func (b *Bridge) Sqlite3ResultErrorNomem(arg0 unsafe.Pointer) {
	Sqlite3ResultErrorNomem(arg0)
}

func (b *Bridge) Sqlite3ResultErrorCode(arg0 unsafe.Pointer, arg1 uintptr) {
	Sqlite3ResultErrorCode(arg0, arg1)
}

func (b *Bridge) Sqlite3ResultInt(arg0 unsafe.Pointer, arg1 uintptr) {
	Sqlite3ResultInt(arg0, arg1)
}

func (b *Bridge) Sqlite3ResultInt64(arg0 unsafe.Pointer, sqlite3Int64 uintptr) {
	Sqlite3ResultInt64(arg0, sqlite3Int64)
}

func (b *Bridge) Sqlite3ResultNull(arg0 unsafe.Pointer) {
	Sqlite3ResultNull(arg0)
}

func (b *Bridge) Sqlite3ResultText(arg0 unsafe.Pointer, arg1 string, arg2 uintptr, arg3 unsafe.Pointer) {
	Sqlite3ResultText(arg0, arg1, arg2, arg3)
}

func (b *Bridge) Sqlite3ResultText64(arg0 unsafe.Pointer, z string, n uint64, arg1 unsafe.Pointer, encoding uintptr) {
	Sqlite3ResultText64(arg0, z, n, arg1, encoding)
}

func (b *Bridge) Sqlite3ResultText16(arg0 unsafe.Pointer, arg1 unsafe.Pointer, arg2 uintptr, arg3 unsafe.Pointer) {
	Sqlite3ResultText16(arg0, arg1, arg2, arg3)
}

func (b *Bridge) Sqlite3ResultText16le(arg0 unsafe.Pointer, arg1 unsafe.Pointer, arg2 uintptr, arg3 unsafe.Pointer) {
	Sqlite3ResultText16le(arg0, arg1, arg2, arg3)
}

func (b *Bridge) Sqlite3ResultText16be(arg0 unsafe.Pointer, arg1 unsafe.Pointer, arg2 uintptr, arg3 unsafe.Pointer) {
	Sqlite3ResultText16be(arg0, arg1, arg2, arg3)
}

func (b *Bridge) Sqlite3ResultValue(arg0 unsafe.Pointer, arg1 unsafe.Pointer) {
	Sqlite3ResultValue(arg0, arg1)
}

func (b *Bridge) Sqlite3ResultPointer(arg0 unsafe.Pointer, arg1 unsafe.Pointer, arg2 string, arg3 unsafe.Pointer) {
	Sqlite3ResultPointer(arg0, arg1, arg2, arg3)
}

func (b *Bridge) Sqlite3ResultZeroblob(arg0 unsafe.Pointer, n int32) {
	Sqlite3ResultZeroblob(arg0, n)
}

func (b *Bridge) Sqlite3ResultZeroblob64(arg0 unsafe.Pointer, n uint64) int32 {
	return Sqlite3ResultZeroblob64(arg0, n)
}

func (b *Bridge) Sqlite3ResultSubtype(arg0 unsafe.Pointer, arg1 uintptr) {
	Sqlite3ResultSubtype(arg0, arg1)
}

// --- VFS ---

func (b *Bridge) Sqlite3VFSFind(zVfsName string) unsafe.Pointer {
	return Sqlite3VFSFind(zVfsName)
}

func (b *Bridge) Sqlite3VFSRegister(arg0 unsafe.Pointer, makeDflt int32) int32 {
	return Sqlite3VFSRegister(arg0, makeDflt)
}

func (b *Bridge) Sqlite3VFSUnregister(arg0 unsafe.Pointer) int32 {
	return Sqlite3VFSUnregister(arg0)
}

// --- Wal ---

func (b *Bridge) Sqlite3WALHook(arg0 unsafe.Pointer, arg1 unsafe.Pointer, arg2 unsafe.Pointer) unsafe.Pointer {
	return Sqlite3WALHook(arg0, arg1, arg2)
}

func (b *Bridge) Sqlite3WALAutocheckpoint(dB unsafe.Pointer, n int32) int32 {
	return Sqlite3WALAutocheckpoint(dB, n)
}

func (b *Bridge) Sqlite3WALCheckpoint(dB unsafe.Pointer, zDb string) int32 {
	return Sqlite3WALCheckpoint(dB, zDb)
}

func (b *Bridge) Sqlite3WALCheckpointV2(dB unsafe.Pointer, zDb string, eMode int32, pnLog unsafe.Pointer, pnCkpt unsafe.Pointer) int32 {
	return Sqlite3WALCheckpointV2(dB, zDb, eMode, pnLog, pnCkpt)
}

// --- Serialize ---

func (b *Bridge) Sqlite3Serialize(dB unsafe.Pointer, zSchema string, piSize int64, mFlags uint32) *byte {
	return Sqlite3Serialize(dB, zSchema, piSize, mFlags)
}

func (b *Bridge) Sqlite3Deserialize(dB unsafe.Pointer, zSchema string, pData *byte, szDb int64, szBuf int64, mFlags uintptr) int32 {
	return Sqlite3Deserialize(dB, zSchema, pData, szDb, szBuf, mFlags)
}

// --- Str ---

func (b *Bridge) Sqlite3StrNew(arg0 unsafe.Pointer) unsafe.Pointer {
	return Sqlite3StrNew(arg0)
}

func (b *Bridge) Sqlite3StrFinish(arg0 unsafe.Pointer) string {
	return Sqlite3StrFinish(arg0)
}

func (b *Bridge) Sqlite3StrFree(arg0 unsafe.Pointer) {
	Sqlite3StrFree(arg0)
}

func (b *Bridge) Sqlite3StrVappendf(arg0 unsafe.Pointer, zFormat string, vaList uintptr) {
	Sqlite3StrVappendf(arg0, zFormat, vaList)
}

func (b *Bridge) Sqlite3StrAppend(arg0 unsafe.Pointer, zIn string, n int32) {
	Sqlite3StrAppend(arg0, zIn, n)
}

func (b *Bridge) Sqlite3StrAppendall(arg0 unsafe.Pointer, zIn string) {
	Sqlite3StrAppendall(arg0, zIn)
}

func (b *Bridge) Sqlite3StrAppendchar(arg0 unsafe.Pointer, n int32, c uintptr) {
	Sqlite3StrAppendchar(arg0, n, c)
}

func (b *Bridge) Sqlite3StrReset(arg0 unsafe.Pointer) {
	Sqlite3StrReset(arg0)
}

func (b *Bridge) Sqlite3StrTruncate(arg0 unsafe.Pointer, n int32) {
	Sqlite3StrTruncate(arg0, n)
}

func (b *Bridge) Sqlite3StrErrcode(arg0 unsafe.Pointer) int32 {
	return Sqlite3StrErrcode(arg0)
}

func (b *Bridge) Sqlite3StrLength(arg0 unsafe.Pointer) int32 {
	return Sqlite3StrLength(arg0)
}

func (b *Bridge) Sqlite3StrValue(arg0 unsafe.Pointer) string {
	return Sqlite3StrValue(arg0)
}

// --- Vtab ---

func (b *Bridge) Sqlite3DeclareVtab(arg0 unsafe.Pointer, zSQL string) int32 {
	return Sqlite3DeclareVtab(arg0, zSQL)
}

func (b *Bridge) Sqlite3OverloadFunction(arg0 unsafe.Pointer, zFuncName string, nArg int32) int32 {
	return Sqlite3OverloadFunction(arg0, zFuncName, nArg)
}

func (b *Bridge) Sqlite3VtabOnConflict(arg0 unsafe.Pointer) int32 {
	return Sqlite3VtabOnConflict(arg0)
}

func (b *Bridge) Sqlite3VtabNochange(arg0 unsafe.Pointer) int32 {
	return Sqlite3VtabNochange(arg0)
}

func (b *Bridge) Sqlite3VtabCollation(arg0 unsafe.Pointer, arg1 uintptr) string {
	return Sqlite3VtabCollation(arg0, arg1)
}

func (b *Bridge) Sqlite3VtabDistinct(arg0 unsafe.Pointer) int32 {
	return Sqlite3VtabDistinct(arg0)
}

func (b *Bridge) Sqlite3VtabIn(arg0 unsafe.Pointer, iCons int32, bHandle int32) int32 {
	return Sqlite3VtabIn(arg0, iCons, bHandle)
}

func (b *Bridge) Sqlite3VtabInFirst(pVal unsafe.Pointer, ppOut unsafe.Pointer) int32 {
	return Sqlite3VtabInFirst(pVal, ppOut)
}

func (b *Bridge) Sqlite3VtabInNext(pVal unsafe.Pointer, ppOut unsafe.Pointer) int32 {
	return Sqlite3VtabInNext(pVal, ppOut)
}

func (b *Bridge) Sqlite3VtabRhsValue(arg0 unsafe.Pointer, arg1 uintptr, ppVal unsafe.Pointer) int32 {
	return Sqlite3VtabRhsValue(arg0, arg1, ppVal)
}

// --- Auth ---

func (b *Bridge) Sqlite3SetAuthorizer(arg0 unsafe.Pointer, arg1 unsafe.Pointer, pUserData unsafe.Pointer) int32 {
	return Sqlite3SetAuthorizer(arg0, arg1, pUserData)
}

// --- Trace ---

func (b *Bridge) Sqlite3Trace(arg0 unsafe.Pointer, arg1 unsafe.Pointer, arg2 unsafe.Pointer) unsafe.Pointer {
	return Sqlite3Trace(arg0, arg1, arg2)
}

func (b *Bridge) Sqlite3Profile(arg0 unsafe.Pointer, arg1 unsafe.Pointer, arg2 unsafe.Pointer) unsafe.Pointer {
	return Sqlite3Profile(arg0, arg1, arg2)
}

func (b *Bridge) Sqlite3TraceV2(arg0 unsafe.Pointer, uMask uintptr, arg1 unsafe.Pointer, pCtx unsafe.Pointer) int32 {
	return Sqlite3TraceV2(arg0, uMask, arg1, pCtx)
}

// --- Hook ---

func (b *Bridge) Sqlite3CommitHook(arg0 unsafe.Pointer, arg1 unsafe.Pointer, arg2 unsafe.Pointer) unsafe.Pointer {
	return Sqlite3CommitHook(arg0, arg1, arg2)
}

func (b *Bridge) Sqlite3RollbackHook(arg0 unsafe.Pointer, arg1 unsafe.Pointer, arg2 unsafe.Pointer) unsafe.Pointer {
	return Sqlite3RollbackHook(arg0, arg1, arg2)
}

func (b *Bridge) Sqlite3AutovacuumPages(dB unsafe.Pointer, arg0 unsafe.Pointer, arg1 unsafe.Pointer, arg2 unsafe.Pointer) int32 {
	return Sqlite3AutovacuumPages(dB, arg0, arg1, arg2)
}

func (b *Bridge) Sqlite3UpdateHook(arg0 unsafe.Pointer, arg1 unsafe.Pointer, arg2 unsafe.Pointer) unsafe.Pointer {
	return Sqlite3UpdateHook(arg0, arg1, arg2)
}

func (b *Bridge) Sqlite3PreupdateHook(dB unsafe.Pointer, arg0 unsafe.Pointer, arg1 unsafe.Pointer) unsafe.Pointer {
	return Sqlite3PreupdateHook(dB, arg0, arg1)
}

func (b *Bridge) Sqlite3PreupdateOld(arg0 unsafe.Pointer, arg1 uintptr, arg2 unsafe.Pointer) int32 {
	return Sqlite3PreupdateOld(arg0, arg1, arg2)
}

func (b *Bridge) Sqlite3PreupdateCount(arg0 unsafe.Pointer) int32 {
	return Sqlite3PreupdateCount(arg0)
}

func (b *Bridge) Sqlite3PreupdateDepth(arg0 unsafe.Pointer) int32 {
	return Sqlite3PreupdateDepth(arg0)
}

func (b *Bridge) Sqlite3PreupdateNew(arg0 unsafe.Pointer, arg1 uintptr, arg2 unsafe.Pointer) int32 {
	return Sqlite3PreupdateNew(arg0, arg1, arg2)
}

func (b *Bridge) Sqlite3PreupdateBlobwrite(arg0 unsafe.Pointer) int32 {
	return Sqlite3PreupdateBlobwrite(arg0)
}

// --- Config ---

func (b *Bridge) Sqlite3CompileoptionUsed(zOptName string) int32 {
	return Sqlite3CompileoptionUsed(zOptName)
}

func (b *Bridge) Sqlite3CompileoptionGet(n int32) string {
	return Sqlite3CompileoptionGet(n)
}

func (b *Bridge) Sqlite3Threadsafe() int32 {
	return Sqlite3Threadsafe()
}

func (b *Bridge) Sqlite3ExtendedResultCodes(arg0 unsafe.Pointer, onoff int32) int32 {
	return Sqlite3ExtendedResultCodes(arg0, onoff)
}

// --- Memory ---

func (b *Bridge) Sqlite3FreeTable(result unsafe.Pointer) {
	Sqlite3FreeTable(result)
}

func (b *Bridge) Sqlite3Malloc(arg0 uintptr) unsafe.Pointer {
	return Sqlite3Malloc(arg0)
}

func (b *Bridge) Sqlite3Malloc64(sqlite3Uint64 uintptr) unsafe.Pointer {
	return Sqlite3Malloc64(sqlite3Uint64)
}

func (b *Bridge) Sqlite3Realloc(arg0 unsafe.Pointer, arg1 uintptr) unsafe.Pointer {
	return Sqlite3Realloc(arg0, arg1)
}

func (b *Bridge) Sqlite3Realloc64(arg0 unsafe.Pointer, sqlite3Uint64 uintptr) unsafe.Pointer {
	return Sqlite3Realloc64(arg0, sqlite3Uint64)
}

func (b *Bridge) Sqlite3Free(arg0 unsafe.Pointer) {
	Sqlite3Free(arg0)
}

func (b *Bridge) Sqlite3Msize(arg0 unsafe.Pointer) uint64 {
	return Sqlite3Msize(arg0)
}

func (b *Bridge) Sqlite3MemoryUsed() int64 {
	return Sqlite3MemoryUsed()
}

func (b *Bridge) Sqlite3MemoryHighwater(resetFlag int32) int64 {
	return Sqlite3MemoryHighwater(resetFlag)
}

func (b *Bridge) Sqlite3FreeFilename(sqlite3Filename uintptr) {
	Sqlite3FreeFilename(sqlite3Filename)
}

func (b *Bridge) Sqlite3ReleaseMemory(arg0 uintptr) int32 {
	return Sqlite3ReleaseMemory(arg0)
}

func (b *Bridge) Sqlite3DBReleaseMemory(arg0 unsafe.Pointer) int32 {
	return Sqlite3DBReleaseMemory(arg0)
}

func (b *Bridge) Sqlite3SoftHeapLimit64(n int64) int64 {
	return Sqlite3SoftHeapLimit64(n)
}

func (b *Bridge) Sqlite3HardHeapLimit64(n int64) int64 {
	return Sqlite3HardHeapLimit64(n)
}

func (b *Bridge) Sqlite3SoftHeapLimit(n int32) {
	Sqlite3SoftHeapLimit(n)
}

// --- Func ---

func (b *Bridge) Sqlite3CreateFunction(dB unsafe.Pointer, zFunctionName string, nArg int32, eTextRep int32, pApp unsafe.Pointer, arg0 unsafe.Pointer, arg1 unsafe.Pointer, arg2 unsafe.Pointer) int32 {
	return Sqlite3CreateFunction(dB, zFunctionName, nArg, eTextRep, pApp, arg0, arg1, arg2)
}

func (b *Bridge) Sqlite3CreateFunction16(dB unsafe.Pointer, zFunctionName unsafe.Pointer, nArg int32, eTextRep int32, pApp unsafe.Pointer, arg0 unsafe.Pointer, arg1 unsafe.Pointer, arg2 unsafe.Pointer) int32 {
	return Sqlite3CreateFunction16(dB, zFunctionName, nArg, eTextRep, pApp, arg0, arg1, arg2)
}

func (b *Bridge) Sqlite3CreateFunctionV2(dB unsafe.Pointer, zFunctionName string, nArg int32, eTextRep int32, pApp unsafe.Pointer, arg0 unsafe.Pointer, arg1 unsafe.Pointer, arg2 unsafe.Pointer, arg3 unsafe.Pointer) int32 {
	return Sqlite3CreateFunctionV2(dB, zFunctionName, nArg, eTextRep, pApp, arg0, arg1, arg2, arg3)
}

func (b *Bridge) Sqlite3CreateWindowFunction(dB unsafe.Pointer, zFunctionName string, nArg int32, eTextRep int32, pApp unsafe.Pointer, arg0 unsafe.Pointer, arg1 unsafe.Pointer, arg2 unsafe.Pointer, arg3 unsafe.Pointer, arg4 unsafe.Pointer) int32 {
	return Sqlite3CreateWindowFunction(dB, zFunctionName, nArg, eTextRep, pApp, arg0, arg1, arg2, arg3, arg4)
}

func (b *Bridge) Sqlite3CreateCollation(arg0 unsafe.Pointer, zName string, eTextRep int32, pArg unsafe.Pointer, arg1 unsafe.Pointer) int32 {
	return Sqlite3CreateCollation(arg0, zName, eTextRep, pArg, arg1)
}

func (b *Bridge) Sqlite3CreateCollationV2(arg0 unsafe.Pointer, zName string, eTextRep int32, pArg unsafe.Pointer, arg1 unsafe.Pointer, arg2 unsafe.Pointer) int32 {
	return Sqlite3CreateCollationV2(arg0, zName, eTextRep, pArg, arg1, arg2)
}

func (b *Bridge) Sqlite3CreateCollation16(arg0 unsafe.Pointer, zName unsafe.Pointer, eTextRep int32, pArg unsafe.Pointer, arg1 unsafe.Pointer) int32 {
	return Sqlite3CreateCollation16(arg0, zName, eTextRep, pArg, arg1)
}

// --- Misc ---

func (b *Bridge) Sqlite3Libversion() string {
	return Sqlite3Libversion()
}

func (b *Bridge) Sqlite3Sourceid() string {
	return Sqlite3Sourceid()
}

func (b *Bridge) Sqlite3LibversionNumber() int32 {
	return Sqlite3LibversionNumber()
}

func (b *Bridge) Sqlite3Complete(sQL string) int32 {
	return Sqlite3Complete(sQL)
}

func (b *Bridge) Sqlite3Complete16(sQL unsafe.Pointer) int32 {
	return Sqlite3Complete16(sQL)
}

func (b *Bridge) Sqlite3BusyHandler(arg0 unsafe.Pointer, arg1 unsafe.Pointer, arg2 unsafe.Pointer) int32 {
	return Sqlite3BusyHandler(arg0, arg1, arg2)
}

func (b *Bridge) Sqlite3BusyTimeout(arg0 unsafe.Pointer, ms int32) int32 {
	return Sqlite3BusyTimeout(arg0, ms)
}

func (b *Bridge) Sqlite3SetlkTimeout(arg0 unsafe.Pointer, ms int32, flags int32) int32 {
	return Sqlite3SetlkTimeout(arg0, ms, flags)
}

func (b *Bridge) Sqlite3GetTable(dB unsafe.Pointer, zSql string, pazResult unsafe.Pointer, pnRow unsafe.Pointer, pnColumn unsafe.Pointer, pzErrmsg unsafe.Pointer) int32 {
	return Sqlite3GetTable(dB, zSql, pazResult, pnRow, pnColumn, pzErrmsg)
}

func (b *Bridge) Sqlite3Vmprintf(arg0 string, vaList uintptr) string {
	return Sqlite3Vmprintf(arg0, vaList)
}

func (b *Bridge) Sqlite3Vsnprintf(arg0 uintptr, arg1 string, arg2 string, vaList uintptr) string {
	return Sqlite3Vsnprintf(arg0, arg1, arg2, vaList)
}

func (b *Bridge) Sqlite3Randomness(n int32, p unsafe.Pointer) {
	Sqlite3Randomness(n, p)
}

func (b *Bridge) Sqlite3ProgressHandler(arg0 unsafe.Pointer, arg1 uintptr, arg2 unsafe.Pointer, arg3 unsafe.Pointer) {
	Sqlite3ProgressHandler(arg0, arg1, arg2, arg3)
}

func (b *Bridge) Sqlite3UriParameter(z uintptr, zParam string) string {
	return Sqlite3UriParameter(z, zParam)
}

func (b *Bridge) Sqlite3UriBoolean(z uintptr, zParam string, bDefault int32) int32 {
	return Sqlite3UriBoolean(z, zParam, bDefault)
}

func (b *Bridge) Sqlite3UriInt64(sqlite3Filename uintptr, arg0 string, sqlite3Int64 uintptr) int64 {
	return Sqlite3UriInt64(sqlite3Filename, arg0, sqlite3Int64)
}

func (b *Bridge) Sqlite3UriKey(z uintptr, n int32) string {
	return Sqlite3UriKey(z, n)
}

func (b *Bridge) Sqlite3FilenameDatabase(sqlite3Filename uintptr) string {
	return Sqlite3FilenameDatabase(sqlite3Filename)
}

func (b *Bridge) Sqlite3FilenameJournal(sqlite3Filename uintptr) string {
	return Sqlite3FilenameJournal(sqlite3Filename)
}

func (b *Bridge) Sqlite3FilenameWAL(sqlite3Filename uintptr) string {
	return Sqlite3FilenameWAL(sqlite3Filename)
}

func (b *Bridge) Sqlite3DatabaseFileObject(arg0 string) unsafe.Pointer {
	return Sqlite3DatabaseFileObject(arg0)
}

func (b *Bridge) Sqlite3CreateFilename(zDatabase string, zJournal string, zWal string, nParam int32, azParam unsafe.Pointer) uintptr {
	return Sqlite3CreateFilename(zDatabase, zJournal, zWal, nParam, azParam)
}

func (b *Bridge) Sqlite3SetErrmsg(dB unsafe.Pointer, errcode int32, zErrMsg string) int32 {
	return Sqlite3SetErrmsg(dB, errcode, zErrMsg)
}

func (b *Bridge) Sqlite3Limit(arg0 unsafe.Pointer, iD int32, newVal int32) int32 {
	return Sqlite3Limit(arg0, iD, newVal)
}

func (b *Bridge) Sqlite3AggregateCount(arg0 unsafe.Pointer) int32 {
	return Sqlite3AggregateCount(arg0)
}

func (b *Bridge) Sqlite3Expired(arg0 unsafe.Pointer) int32 {
	return Sqlite3Expired(arg0)
}

func (b *Bridge) Sqlite3TransferBindings(arg0 unsafe.Pointer, arg1 unsafe.Pointer) int32 {
	return Sqlite3TransferBindings(arg0, arg1)
}

func (b *Bridge) Sqlite3GlobalRecover() int32 {
	return Sqlite3GlobalRecover()
}

func (b *Bridge) Sqlite3ThreadCleanup() {
	Sqlite3ThreadCleanup()
}

func (b *Bridge) Sqlite3MemoryAlarm(arg0 unsafe.Pointer, arg1 unsafe.Pointer, sqlite3Int64 uintptr) int32 {
	return Sqlite3MemoryAlarm(arg0, arg1, sqlite3Int64)
}

func (b *Bridge) Sqlite3UserData(arg0 unsafe.Pointer) unsafe.Pointer {
	return Sqlite3UserData(arg0)
}

func (b *Bridge) Sqlite3GetAuxdata(arg0 unsafe.Pointer, n int32) unsafe.Pointer {
	return Sqlite3GetAuxdata(arg0, n)
}

func (b *Bridge) Sqlite3SetAuxdata(arg0 unsafe.Pointer, n int32, arg1 unsafe.Pointer, arg2 unsafe.Pointer) {
	Sqlite3SetAuxdata(arg0, n, arg1, arg2)
}

func (b *Bridge) Sqlite3GetClientdata(arg0 unsafe.Pointer, arg1 string) unsafe.Pointer {
	return Sqlite3GetClientdata(arg0, arg1)
}

func (b *Bridge) Sqlite3SetClientdata(arg0 unsafe.Pointer, arg1 string, arg2 unsafe.Pointer, arg3 unsafe.Pointer) int32 {
	return Sqlite3SetClientdata(arg0, arg1, arg2, arg3)
}

func (b *Bridge) Sqlite3CollationNeeded(arg0 unsafe.Pointer, arg1 unsafe.Pointer, arg2 unsafe.Pointer) int32 {
	return Sqlite3CollationNeeded(arg0, arg1, arg2)
}

func (b *Bridge) Sqlite3CollationNeeded16(arg0 unsafe.Pointer, arg1 unsafe.Pointer, arg2 unsafe.Pointer) int32 {
	return Sqlite3CollationNeeded16(arg0, arg1, arg2)
}

func (b *Bridge) Sqlite3ActivateCerod(zPassPhrase string) {
	Sqlite3ActivateCerod(zPassPhrase)
}

func (b *Bridge) Sqlite3Sleep(arg0 uintptr) int32 {
	return Sqlite3Sleep(arg0)
}

func (b *Bridge) Sqlite3Win32SetDirectory(arg0 uintptr, zValue unsafe.Pointer) int32 {
	return Sqlite3Win32SetDirectory(arg0, zValue)
}

func (b *Bridge) Sqlite3Win32SetDirectory8(arg0 uintptr, zValue string) int32 {
	return Sqlite3Win32SetDirectory8(arg0, zValue)
}

func (b *Bridge) Sqlite3Win32SetDirectory16(arg0 uintptr, zValue unsafe.Pointer) int32 {
	return Sqlite3Win32SetDirectory16(arg0, zValue)
}

func (b *Bridge) Sqlite3GetAutocommit(arg0 unsafe.Pointer) int32 {
	return Sqlite3GetAutocommit(arg0)
}

func (b *Bridge) Sqlite3DBHandle(arg0 unsafe.Pointer) unsafe.Pointer {
	return Sqlite3DBHandle(arg0)
}

func (b *Bridge) Sqlite3DBName(dB unsafe.Pointer, n int32) string {
	return Sqlite3DBName(dB, n)
}

func (b *Bridge) Sqlite3DBFilename(dB unsafe.Pointer, zDbName string) uintptr {
	return Sqlite3DBFilename(dB, zDbName)
}

func (b *Bridge) Sqlite3DBReadonly(dB unsafe.Pointer, zDbName string) int32 {
	return Sqlite3DBReadonly(dB, zDbName)
}

func (b *Bridge) Sqlite3TxnState(arg0 unsafe.Pointer, zSchema string) int32 {
	return Sqlite3TxnState(arg0, zSchema)
}

func (b *Bridge) Sqlite3NextStmt(pDb unsafe.Pointer, pStmt unsafe.Pointer) unsafe.Pointer {
	return Sqlite3NextStmt(pDb, pStmt)
}

func (b *Bridge) Sqlite3EnableSharedCache(arg0 uintptr) int32 {
	return Sqlite3EnableSharedCache(arg0)
}

func (b *Bridge) Sqlite3TableColumnMetadata(dB unsafe.Pointer, zDbName string, zTableName string, zColumnName string, pzDataType unsafe.Pointer, pzCollSeq unsafe.Pointer, pNotNull unsafe.Pointer, pPrimaryKey unsafe.Pointer, pAutoinc unsafe.Pointer) int32 {
	return Sqlite3TableColumnMetadata(dB, zDbName, zTableName, zColumnName, pzDataType, pzCollSeq, pNotNull, pPrimaryKey, pAutoinc)
}

func (b *Bridge) Sqlite3LoadExtension(dB unsafe.Pointer, zFile string, zProc string, pzErrMsg unsafe.Pointer) int32 {
	return Sqlite3LoadExtension(dB, zFile, zProc, pzErrMsg)
}

func (b *Bridge) Sqlite3EnableLoadExtension(dB unsafe.Pointer, onoff int32) int32 {
	return Sqlite3EnableLoadExtension(dB, onoff)
}

func (b *Bridge) Sqlite3AutoExtension(arg0 unsafe.Pointer) int32 {
	return Sqlite3AutoExtension(arg0)
}

func (b *Bridge) Sqlite3CancelAutoExtension(arg0 unsafe.Pointer) int32 {
	return Sqlite3CancelAutoExtension(arg0)
}

func (b *Bridge) Sqlite3CreateModule(dB unsafe.Pointer, zName string, p unsafe.Pointer, pClientData unsafe.Pointer) int32 {
	return Sqlite3CreateModule(dB, zName, p, pClientData)
}

func (b *Bridge) Sqlite3CreateModuleV2(dB unsafe.Pointer, zName string, p unsafe.Pointer, pClientData unsafe.Pointer, arg0 unsafe.Pointer) int32 {
	return Sqlite3CreateModuleV2(dB, zName, p, pClientData, arg0)
}

func (b *Bridge) Sqlite3DropModules(dB unsafe.Pointer, azKeep unsafe.Pointer) int32 {
	return Sqlite3DropModules(dB, azKeep)
}

func (b *Bridge) Sqlite3MutexAlloc(arg0 uintptr) unsafe.Pointer {
	return Sqlite3MutexAlloc(arg0)
}

func (b *Bridge) Sqlite3MutexFree(arg0 unsafe.Pointer) {
	Sqlite3MutexFree(arg0)
}

func (b *Bridge) Sqlite3MutexEnter(arg0 unsafe.Pointer) {
	Sqlite3MutexEnter(arg0)
}

func (b *Bridge) Sqlite3MutexTry(arg0 unsafe.Pointer) int32 {
	return Sqlite3MutexTry(arg0)
}

func (b *Bridge) Sqlite3MutexLeave(arg0 unsafe.Pointer) {
	Sqlite3MutexLeave(arg0)
}

func (b *Bridge) Sqlite3MutexHeld(arg0 unsafe.Pointer) int32 {
	return Sqlite3MutexHeld(arg0)
}

func (b *Bridge) Sqlite3MutexNotheld(arg0 unsafe.Pointer) int32 {
	return Sqlite3MutexNotheld(arg0)
}

func (b *Bridge) Sqlite3DBMutex(arg0 unsafe.Pointer) unsafe.Pointer {
	return Sqlite3DBMutex(arg0)
}

func (b *Bridge) Sqlite3FileControl(arg0 unsafe.Pointer, zDbName string, op int32, arg1 unsafe.Pointer) int32 {
	return Sqlite3FileControl(arg0, zDbName, op, arg1)
}

func (b *Bridge) Sqlite3KeywordCount() int32 {
	return Sqlite3KeywordCount()
}

func (b *Bridge) Sqlite3KeywordName(arg0 uintptr, arg1 unsafe.Pointer, arg2 unsafe.Pointer) int32 {
	return Sqlite3KeywordName(arg0, arg1, arg2)
}

func (b *Bridge) Sqlite3KeywordCheck(arg0 string, arg1 uintptr) int32 {
	return Sqlite3KeywordCheck(arg0, arg1)
}

func (b *Bridge) Sqlite3Status(op int32, pCurrent unsafe.Pointer, pHighwater unsafe.Pointer, resetFlag int32) int32 {
	return Sqlite3Status(op, pCurrent, pHighwater, resetFlag)
}

func (b *Bridge) Sqlite3Status64(op int32, pCurrent int64, pHighwater int64, resetFlag int32) int32 {
	return Sqlite3Status64(op, pCurrent, pHighwater, resetFlag)
}

func (b *Bridge) Sqlite3DBStatus(arg0 unsafe.Pointer, op int32, pCur unsafe.Pointer, pHiwtr unsafe.Pointer, resetFlg int32) int32 {
	return Sqlite3DBStatus(arg0, op, pCur, pHiwtr, resetFlg)
}

func (b *Bridge) Sqlite3DBStatus64(arg0 unsafe.Pointer, arg1 uintptr, arg2 int64, arg3 int64, arg4 uintptr) int32 {
	return Sqlite3DBStatus64(arg0, arg1, arg2, arg3, arg4)
}

func (b *Bridge) Sqlite3UnlockNotify(pBlocked unsafe.Pointer, arg0 unsafe.Pointer, pNotifyArg unsafe.Pointer) int32 {
	return Sqlite3UnlockNotify(pBlocked, arg0, pNotifyArg)
}

func (b *Bridge) Sqlite3Stricmp(arg0 string, arg1 string) int32 {
	return Sqlite3Stricmp(arg0, arg1)
}

func (b *Bridge) Sqlite3Strnicmp(arg0 string, arg1 string, arg2 uintptr) int32 {
	return Sqlite3Strnicmp(arg0, arg1, arg2)
}

func (b *Bridge) Sqlite3Strglob(zGlob string, zStr string) int32 {
	return Sqlite3Strglob(zGlob, zStr)
}

func (b *Bridge) Sqlite3Strlike(zGlob string, zStr string, cEsc uint32) int32 {
	return Sqlite3Strlike(zGlob, zStr, cEsc)
}

func (b *Bridge) Sqlite3DBCacheflush(arg0 unsafe.Pointer) int32 {
	return Sqlite3DBCacheflush(arg0)
}

func (b *Bridge) Sqlite3SystemErrno(arg0 unsafe.Pointer) int32 {
	return Sqlite3SystemErrno(arg0)
}

func (b *Bridge) Sqlite3SnapshotGet(dB unsafe.Pointer, zSchema string, ppSnapshot unsafe.Pointer) int32 {
	return Sqlite3SnapshotGet(dB, zSchema, ppSnapshot)
}

func (b *Bridge) Sqlite3SnapshotOpen(dB unsafe.Pointer, zSchema string, pSnapshot unsafe.Pointer) int32 {
	return Sqlite3SnapshotOpen(dB, zSchema, pSnapshot)
}

func (b *Bridge) Sqlite3SnapshotFree(arg0 unsafe.Pointer) {
	Sqlite3SnapshotFree(arg0)
}

func (b *Bridge) Sqlite3SnapshotCmp(p1 unsafe.Pointer, p2 unsafe.Pointer) int32 {
	return Sqlite3SnapshotCmp(p1, p2)
}

func (b *Bridge) Sqlite3SnapshotRecover(dB unsafe.Pointer, zDb string) int32 {
	return Sqlite3SnapshotRecover(dB, zDb)
}

func (b *Bridge) Sqlite3CarrayBindV2(pStmt unsafe.Pointer, i int32, aData unsafe.Pointer, nData int32, mFlags int32, arg0 unsafe.Pointer, pDel unsafe.Pointer) int32 {
	return Sqlite3CarrayBindV2(pStmt, i, aData, nData, mFlags, arg0, pDel)
}

func (b *Bridge) Sqlite3CarrayBind(pStmt unsafe.Pointer, i int32, aData unsafe.Pointer, nData int32, mFlags int32, arg0 unsafe.Pointer) int32 {
	return Sqlite3CarrayBind(pStmt, i, aData, nData, mFlags, arg0)
}

func (b *Bridge) Sqlite3RtreeGeometryCallback(dB unsafe.Pointer, zGeom string, arg0 unsafe.Pointer, pContext unsafe.Pointer) int32 {
	return Sqlite3RtreeGeometryCallback(dB, zGeom, arg0, pContext)
}

func (b *Bridge) Sqlite3RtreeQueryCallback(dB unsafe.Pointer, zQueryFunc string, arg0 unsafe.Pointer, pContext unsafe.Pointer, arg1 unsafe.Pointer) int32 {
	return Sqlite3RtreeQueryCallback(dB, zQueryFunc, arg0, pContext, arg1)
}

func (b *Bridge) Sqlite3sessionCreate(dB unsafe.Pointer, zDb string, ppSession unsafe.Pointer) int32 {
	return Sqlite3sessionCreate(dB, zDb, ppSession)
}

func (b *Bridge) Sqlite3sessionDelete(pSession unsafe.Pointer) {
	Sqlite3sessionDelete(pSession)
}

func (b *Bridge) Sqlite3sessionObjectConfig(arg0 unsafe.Pointer, op int32, pArg unsafe.Pointer) int32 {
	return Sqlite3sessionObjectConfig(arg0, op, pArg)
}

func (b *Bridge) Sqlite3sessionEnable(pSession unsafe.Pointer, bEnable int32) int32 {
	return Sqlite3sessionEnable(pSession, bEnable)
}

func (b *Bridge) Sqlite3sessionIndirect(pSession unsafe.Pointer, bIndirect int32) int32 {
	return Sqlite3sessionIndirect(pSession, bIndirect)
}

func (b *Bridge) Sqlite3sessionAttach(pSession unsafe.Pointer, zTab string) int32 {
	return Sqlite3sessionAttach(pSession, zTab)
}

func (b *Bridge) Sqlite3sessionTableFilter(pSession unsafe.Pointer, arg0 unsafe.Pointer, pCtx unsafe.Pointer) {
	Sqlite3sessionTableFilter(pSession, arg0, pCtx)
}

func (b *Bridge) Sqlite3sessionChangeset(pSession unsafe.Pointer, pnChangeset unsafe.Pointer, ppChangeset unsafe.Pointer) int32 {
	return Sqlite3sessionChangeset(pSession, pnChangeset, ppChangeset)
}

func (b *Bridge) Sqlite3sessionChangesetSize(pSession unsafe.Pointer) int64 {
	return Sqlite3sessionChangesetSize(pSession)
}

func (b *Bridge) Sqlite3sessionDiff(pSession unsafe.Pointer, zFromDb string, zTbl string, pzErrMsg unsafe.Pointer) int32 {
	return Sqlite3sessionDiff(pSession, zFromDb, zTbl, pzErrMsg)
}

func (b *Bridge) Sqlite3sessionPatchset(pSession unsafe.Pointer, pnPatchset unsafe.Pointer, ppPatchset unsafe.Pointer) int32 {
	return Sqlite3sessionPatchset(pSession, pnPatchset, ppPatchset)
}

func (b *Bridge) Sqlite3sessionIsempty(pSession unsafe.Pointer) int32 {
	return Sqlite3sessionIsempty(pSession)
}

func (b *Bridge) Sqlite3sessionMemoryUsed(pSession unsafe.Pointer) int64 {
	return Sqlite3sessionMemoryUsed(pSession)
}

func (b *Bridge) Sqlite3changesetStart(pp unsafe.Pointer, nChangeset int32, pChangeset unsafe.Pointer) int32 {
	return Sqlite3changesetStart(pp, nChangeset, pChangeset)
}

func (b *Bridge) Sqlite3changesetStartV2(pp unsafe.Pointer, nChangeset int32, pChangeset unsafe.Pointer, flags int32) int32 {
	return Sqlite3changesetStartV2(pp, nChangeset, pChangeset, flags)
}

func (b *Bridge) Sqlite3changesetNext(pIter unsafe.Pointer) int32 {
	return Sqlite3changesetNext(pIter)
}

func (b *Bridge) Sqlite3changesetOp(pIter unsafe.Pointer, pzTab unsafe.Pointer, pnCol unsafe.Pointer, pOp unsafe.Pointer, pbIndirect unsafe.Pointer) int32 {
	return Sqlite3changesetOp(pIter, pzTab, pnCol, pOp, pbIndirect)
}

func (b *Bridge) Sqlite3changesetPk(pIter unsafe.Pointer, pabPK unsafe.Pointer, pnCol unsafe.Pointer) int32 {
	return Sqlite3changesetPk(pIter, pabPK, pnCol)
}

func (b *Bridge) Sqlite3changesetOld(pIter unsafe.Pointer, iVal int32, ppValue unsafe.Pointer) int32 {
	return Sqlite3changesetOld(pIter, iVal, ppValue)
}

func (b *Bridge) Sqlite3changesetNew(pIter unsafe.Pointer, iVal int32, ppValue unsafe.Pointer) int32 {
	return Sqlite3changesetNew(pIter, iVal, ppValue)
}

func (b *Bridge) Sqlite3changesetConflict(pIter unsafe.Pointer, iVal int32, ppValue unsafe.Pointer) int32 {
	return Sqlite3changesetConflict(pIter, iVal, ppValue)
}

func (b *Bridge) Sqlite3changesetFkConflicts(pIter unsafe.Pointer, pnOut unsafe.Pointer) int32 {
	return Sqlite3changesetFkConflicts(pIter, pnOut)
}

func (b *Bridge) Sqlite3changesetFinalize(pIter unsafe.Pointer) int32 {
	return Sqlite3changesetFinalize(pIter)
}

func (b *Bridge) Sqlite3changesetInvert(nIn int32, pIn unsafe.Pointer, pnOut unsafe.Pointer, ppOut unsafe.Pointer) int32 {
	return Sqlite3changesetInvert(nIn, pIn, pnOut, ppOut)
}

func (b *Bridge) Sqlite3changesetConcat(nA int32, pA unsafe.Pointer, nB int32, pB unsafe.Pointer, pnOut unsafe.Pointer, ppOut unsafe.Pointer) int32 {
	return Sqlite3changesetConcat(nA, pA, nB, pB, pnOut, ppOut)
}

func (b *Bridge) Sqlite3changegroupNew(pp unsafe.Pointer) int32 {
	return Sqlite3changegroupNew(pp)
}

func (b *Bridge) Sqlite3changegroupSchema(arg0 unsafe.Pointer, arg1 unsafe.Pointer, zDb string) int32 {
	return Sqlite3changegroupSchema(arg0, arg1, zDb)
}

func (b *Bridge) Sqlite3changegroupAdd(arg0 unsafe.Pointer, nData int32, pData unsafe.Pointer) int32 {
	return Sqlite3changegroupAdd(arg0, nData, pData)
}

func (b *Bridge) Sqlite3changegroupAddChange(arg0 unsafe.Pointer, arg1 unsafe.Pointer) int32 {
	return Sqlite3changegroupAddChange(arg0, arg1)
}

func (b *Bridge) Sqlite3changegroupOutput(arg0 unsafe.Pointer, pnData unsafe.Pointer, ppData unsafe.Pointer) int32 {
	return Sqlite3changegroupOutput(arg0, pnData, ppData)
}

func (b *Bridge) Sqlite3changegroupDelete(arg0 unsafe.Pointer) {
	Sqlite3changegroupDelete(arg0)
}

func (b *Bridge) Sqlite3changesetApply(dB unsafe.Pointer, nChangeset int32, pChangeset unsafe.Pointer, arg0 unsafe.Pointer, arg1 unsafe.Pointer, pCtx unsafe.Pointer) int32 {
	return Sqlite3changesetApply(dB, nChangeset, pChangeset, arg0, arg1, pCtx)
}

func (b *Bridge) Sqlite3changesetApplyV2(dB unsafe.Pointer, nChangeset int32, pChangeset unsafe.Pointer, arg0 unsafe.Pointer, arg1 unsafe.Pointer, pCtx unsafe.Pointer, ppRebase unsafe.Pointer, pnRebase unsafe.Pointer, flags int32) int32 {
	return Sqlite3changesetApplyV2(dB, nChangeset, pChangeset, arg0, arg1, pCtx, ppRebase, pnRebase, flags)
}

func (b *Bridge) Sqlite3changesetApplyV3(dB unsafe.Pointer, nChangeset int32, pChangeset unsafe.Pointer, arg0 unsafe.Pointer, arg1 unsafe.Pointer, pCtx unsafe.Pointer, ppRebase unsafe.Pointer, pnRebase unsafe.Pointer, flags int32) int32 {
	return Sqlite3changesetApplyV3(dB, nChangeset, pChangeset, arg0, arg1, pCtx, ppRebase, pnRebase, flags)
}

func (b *Bridge) Sqlite3rebaserCreate(ppNew unsafe.Pointer) int32 {
	return Sqlite3rebaserCreate(ppNew)
}

func (b *Bridge) Sqlite3rebaserConfigure(arg0 unsafe.Pointer, nRebase int32, pRebase unsafe.Pointer) int32 {
	return Sqlite3rebaserConfigure(arg0, nRebase, pRebase)
}

func (b *Bridge) Sqlite3rebaserRebase(arg0 unsafe.Pointer, nIn int32, pIn unsafe.Pointer, pnOut unsafe.Pointer, ppOut unsafe.Pointer) int32 {
	return Sqlite3rebaserRebase(arg0, nIn, pIn, pnOut, ppOut)
}

func (b *Bridge) Sqlite3rebaserDelete(p unsafe.Pointer) {
	Sqlite3rebaserDelete(p)
}

func (b *Bridge) Sqlite3changesetApplyStrm(dB unsafe.Pointer, arg0 unsafe.Pointer, pIn unsafe.Pointer, arg1 unsafe.Pointer, arg2 unsafe.Pointer, pCtx unsafe.Pointer) int32 {
	return Sqlite3changesetApplyStrm(dB, arg0, pIn, arg1, arg2, pCtx)
}

func (b *Bridge) Sqlite3changesetApplyV2Strm(dB unsafe.Pointer, arg0 unsafe.Pointer, pIn unsafe.Pointer, arg1 unsafe.Pointer, arg2 unsafe.Pointer, pCtx unsafe.Pointer, ppRebase unsafe.Pointer, pnRebase unsafe.Pointer, flags int32) int32 {
	return Sqlite3changesetApplyV2Strm(dB, arg0, pIn, arg1, arg2, pCtx, ppRebase, pnRebase, flags)
}

func (b *Bridge) Sqlite3changesetApplyV3Strm(dB unsafe.Pointer, arg0 unsafe.Pointer, pIn unsafe.Pointer, arg1 unsafe.Pointer, arg2 unsafe.Pointer, pCtx unsafe.Pointer, ppRebase unsafe.Pointer, pnRebase unsafe.Pointer, flags int32) int32 {
	return Sqlite3changesetApplyV3Strm(dB, arg0, pIn, arg1, arg2, pCtx, ppRebase, pnRebase, flags)
}

func (b *Bridge) Sqlite3changesetConcatStrm(arg0 unsafe.Pointer, pInA unsafe.Pointer, arg1 unsafe.Pointer, pInB unsafe.Pointer, arg2 unsafe.Pointer, pOut unsafe.Pointer) int32 {
	return Sqlite3changesetConcatStrm(arg0, pInA, arg1, pInB, arg2, pOut)
}

func (b *Bridge) Sqlite3changesetInvertStrm(arg0 unsafe.Pointer, pIn unsafe.Pointer, arg1 unsafe.Pointer, pOut unsafe.Pointer) int32 {
	return Sqlite3changesetInvertStrm(arg0, pIn, arg1, pOut)
}

func (b *Bridge) Sqlite3changesetStartStrm(pp unsafe.Pointer, arg0 unsafe.Pointer, pIn unsafe.Pointer) int32 {
	return Sqlite3changesetStartStrm(pp, arg0, pIn)
}

func (b *Bridge) Sqlite3changesetStartV2Strm(pp unsafe.Pointer, arg0 unsafe.Pointer, pIn unsafe.Pointer, flags int32) int32 {
	return Sqlite3changesetStartV2Strm(pp, arg0, pIn, flags)
}

func (b *Bridge) Sqlite3sessionChangesetStrm(pSession unsafe.Pointer, arg0 unsafe.Pointer, pOut unsafe.Pointer) int32 {
	return Sqlite3sessionChangesetStrm(pSession, arg0, pOut)
}

func (b *Bridge) Sqlite3sessionPatchsetStrm(pSession unsafe.Pointer, arg0 unsafe.Pointer, pOut unsafe.Pointer) int32 {
	return Sqlite3sessionPatchsetStrm(pSession, arg0, pOut)
}

func (b *Bridge) Sqlite3changegroupAddStrm(arg0 unsafe.Pointer, arg1 unsafe.Pointer, pIn unsafe.Pointer) int32 {
	return Sqlite3changegroupAddStrm(arg0, arg1, pIn)
}

func (b *Bridge) Sqlite3changegroupOutputStrm(arg0 unsafe.Pointer, arg1 unsafe.Pointer, pOut unsafe.Pointer) int32 {
	return Sqlite3changegroupOutputStrm(arg0, arg1, pOut)
}

func (b *Bridge) Sqlite3rebaserRebaseStrm(pRebaser unsafe.Pointer, arg0 unsafe.Pointer, pIn unsafe.Pointer, arg1 unsafe.Pointer, pOut unsafe.Pointer) int32 {
	return Sqlite3rebaserRebaseStrm(pRebaser, arg0, pIn, arg1, pOut)
}

func (b *Bridge) Sqlite3sessionConfig(op int32, pArg unsafe.Pointer) int32 {
	return Sqlite3sessionConfig(op, pArg)
}
