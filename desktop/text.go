package desktop

import (
	"fmt"
	"os"
	"strings"

	"github.com/oneo-me/nsgo/core"
)

// LangEnvVar picks the language of the window. A person sets it when the
// system language is not the one they read.
const LangEnvVar = "EXODRIVE_LANG"

// text holds every word the window writes. A field exists for each sentence,
// so a language that misses one does not compile.
type text struct {
	// Header.
	WelcomeTitle        string
	WelcomeStepOne      string
	WelcomeStepTwo      string
	WelcomeStepThree    string
	WelcomeStart        string
	WelcomeSkip         string
	Subtitle            string
	CheckingThisMac     string
	CheckingThisMacLine string
	CheckPromise        string

	// Gauges.
	ThisMac           string
	ExternalDrive     string
	FreeUnknown       string
	FreeAmount        string // %s free
	NoCachesFound     string
	InCaches          string // %s in caches
	NoFolderChosen    string
	NotReady          string
	FolderFailedCheck string
	MovedHere         string // %s moved here
	NoCachesMovedYet  string
	NoExternalDrive   string
	DriveConnected    string // %s is connected.
	DrivesConnected   string // %d drives are connected.
	DriveChooseFolder string // %s: choose a folder to move caches here.
	DriveMostFree     string // %s has the most free space. %d more connected. Choose a folder.
	FolderOnThisMac   string
	FolderPlaceholder string
	Choose            string

	// The action panel.
	ChooseAnotherFolder  string
	ChooseAnotherHelper  string
	ChooseFolderTitle    string
	ChooseFolderHelper   string
	NoCachesInView       string
	NoCachesInViewHelper string
	NothingCanMove       string
	NothingCanMoveHelper string
	SelectCaches         string
	SelectCachesHelper   string
	MoveTitle            string // Move %s
	MoveHelper           string // %s moves to the drive. Exodrive checks each copy before it frees space.
	ChooseFolderButton   string
	SelectAll            string // Select all %d
	MoveButton           string // Move %s
	CacheOne             string
	CacheMany            string

	// Progress.
	Moving             string // Moving %s
	CheckingCopy       string // Checking the copy of %s
	CacheOfDone        string // Cache %d of %d. %.0f%% done.
	MovingCaches       string
	Finishing          string // Finishing %s
	StoppingAfter      string
	StopAfterThisCache string

	// List header and rail.
	Caches            string
	Selected          string // %d selected
	Clear             string
	Rescan            string
	Place             string
	All               string
	OnThisMac         string
	OnTheDrive        string
	Detected          string
	AllCaches         string
	RailLabel         string // %s, %s
	FamilyOther       string
	EmptyCheckList    string
	EmptyThisView     string
	EmptyThisViewHelp string

	// Cards.
	Folder         string
	Copy           string
	Move           string
	MoveBack       string
	DeleteCache    string
	TryAgain       string
	MoveFailed     string
	MoveBackFailed string
	CannotMove     string
	Unused         string
	MoveStopped    string
	UnusedNote     string
	SwitchNative   string
	SwitchLink     string
	SwitchManual   string

	// Footer.
	Details         string
	HideDetails     string
	MoveRecords     string
	MoveRecordsHelp string

	// Dialogs.
	AndMore                 string // and %d more
	MoveQuestion            string // Move %s?
	MoveDescription         string // %s moves to %s. Exodrive checks each copy, then frees the space on this Mac.
	Cancel                  string
	NothingMoved            string
	MoveBackQuestion        string
	MoveBackDescription     string // Exodrive copies %s from %s to its old folder...
	FromTheDrive            string
	DeleteQuestion          string
	DeleteDescription       string // Exodrive deletes %s from this Mac...
	DeleteDescriptionNoPath string
	NothingDeleted          string

	// Status and notices.
	InterruptedTitle   string
	InterruptedBody    string // %s was moving when Exodrive stopped.
	InterruptedBodyAny string
	InterruptedHint    string
	NoFolderPathTitle  string
	NoFolderPathBody   string
	NoFolderPathHint   string
	CheckingCaches     string
	CheckFailedTitle   string
	CheckPartialTitle  string
	CheckPartialBody   string
	CheckPartialHint   string
	FoundCaches        string // Exodrive found %s. %d can move.
	ChooseFirstTitle   string
	ChooseFirstBody    string
	ChooseFirstHint    string
	MovingStatus       string // Moving %s...
	StoppedStatus      string // Stopped. %s moved. The other caches stay on this Mac.
	SomeMovedStatus    string // %s moved. %s did not move.
	SomeMovedTitleOne  string
	SomeMovedTitleMany string
	SomeMovedBody      string
	SomeMovedHint      string
	MovedStatus        string // Moved %s. %s is now free on this Mac.
	NotInListStatus    string
	DeletingStatus     string // Deleting %s...
	DeletedStatus      string // Deleted %s. %s is now free.
	RollingBackStatus  string
	RolledBackStatus   string
	RetryHint          string // Exodrive deleted nothing. Try again.
	PickerFailedTitle  string
	PickerFailedHint   string

	// Failures.
	MoveStoppedTitle    string
	MoveBackStopped     string
	DeleteStopped       string
	DriveUnsupported    string
	DriveUnsupportedWhy string
	DriveUnsupportedFix string
	DriveFull           string
	DriveFullWhy        string
	DriveFullFix        string
	StillRunning        string
	StillRunningWhy     string
	StillRunningFix     string
	AlreadyLink         string
	AlreadyLinkWhy      string
	AlreadyLinkFix      string
	CopyMismatch        string
	CopyMismatchWhy     string
	CopyMismatchFix     string
	ShellVariable       string
	ShellVariableWhy    string
	ShellVariableFix    string // Remove %s from your shell...
	NoRecord            string
	NoRecordWhy         string
	NoRecordFix         string
	LedgerUnreadable    string
	LedgerUnreadableWhy string
	LedgerUnreadableFix string
	TwoCopies           string
	TwoCopiesWhy        string
	TwoCopiesFix        string
	FolderGone          string
	FolderGoneWhy       string
	FolderGoneFix       string

	// Folder problems.
	DriveCannotHold      string
	TargetMissingMessage string
	VolumeMissing        string // The external volume %s is not connected.
	VolumeMissingFix     string
	FolderMissing        string // The folder %s does not exist.
	FolderMissingFix     string
}

var englishText = text{
	WelcomeTitle:        "Three steps move a cache",
	WelcomeStepOne:      "Choose a folder on the external drive.",
	WelcomeStepTwo:      "Select the caches to move.",
	WelcomeStepThree:    "Press Move. A cache frees its space after the copy passes the check.",
	WelcomeStart:        "Choose folder",
	WelcomeSkip:         "Skip",
	Subtitle:            "Move developer caches to an external drive.",
	CheckingThisMac:     "Checking this Mac",
	CheckingThisMacLine: "Checking this Mac...",
	CheckPromise:        "A cache frees its space only after the copy passes the check.",

	ThisMac:           "This Mac",
	ExternalDrive:     "External drive",
	FreeUnknown:       "Free space unknown",
	FreeAmount:        "%s free",
	NoCachesFound:     "No caches found",
	InCaches:          "%s in caches",
	NoFolderChosen:    "No folder chosen",
	NotReady:          "Not ready",
	FolderFailedCheck: "The folder failed the check.",
	MovedHere:         "%s moved here",
	NoCachesMovedYet:  "No caches moved yet",
	NoExternalDrive:   "No external drive is connected.",
	DriveConnected:    "%s is connected.",
	DrivesConnected:   "%d drives are connected.",
	DriveChooseFolder: "%s: choose a folder to move caches here.",
	DriveMostFree:     "%s has the most free space. %d more connected. Choose a folder.",
	FolderOnThisMac:   "This folder is on this Mac. A move here frees no space.",
	FolderPlaceholder: "Folder on the drive",
	Choose:            "Choose",

	ChooseAnotherFolder:  "Choose another folder",
	ChooseAnotherHelper:  "Exodrive checked the folder and cannot use it.",
	ChooseFolderTitle:    "Choose a folder for the caches",
	ChooseFolderHelper:   "The caches move to this folder. Keep the drive connected.",
	NoCachesInView:       "No caches in this view",
	NoCachesInViewHelper: "Choose All, or another group in the list on the left.",
	NothingCanMove:       "Nothing can move now",
	NothingCanMoveHelper: "Every cache is already on the drive, or it cannot move.",
	SelectCaches:         "Select the caches to move",
	SelectCachesHelper:   "A cache stays on this Mac until the copy passes the check.",
	MoveTitle:            "Move %s",
	MoveHelper:           "%s moves to the drive. Exodrive checks each copy before it frees space.",
	ChooseFolderButton:   "Choose folder",
	SelectAll:            "Select all %d",
	MoveButton:           "Move %s",
	CacheOne:             "cache",
	CacheMany:            "caches",

	Moving:             "Moving %s",
	CheckingCopy:       "Checking the copy of %s",
	CacheOfDone:        "Cache %d of %d. %.0f%% done.",
	MovingCaches:       "Moving the caches",
	Finishing:          "Finishing %s",
	StoppingAfter:      "Stopping after this cache.",
	StopAfterThisCache: "Stop after this cache",

	Caches:            "Caches",
	Selected:          "%d selected",
	Clear:             "Clear",
	Rescan:            "Rescan",
	Place:             "Place",
	All:               "All",
	OnThisMac:         "On this Mac",
	OnTheDrive:        "On the drive",
	Detected:          "Detected",
	AllCaches:         "All caches",
	RailLabel:         "%s, %s",
	FamilyOther:       "Other",
	EmptyCheckList:    "Exodrive checks npm, pnpm, yarn, cargo, Go, bun, uv, pip, and Docker.",
	EmptyThisView:     "No caches in this view",
	EmptyThisViewHelp: "Choose All, or another group in the list on the left.",

	Folder:         "Folder",
	Copy:           "Copy",
	Move:           "Move",
	MoveBack:       "Move back",
	DeleteCache:    "Delete cache",
	TryAgain:       "Try again",
	MoveFailed:     "Move failed",
	MoveBackFailed: "Move back failed",
	CannotMove:     "Cannot move",
	Unused:         "Unused",
	MoveStopped:    "The move below stopped. Read the reason, then try again.",
	UnusedNote:     "The program does not use this cache anymore.",
	SwitchNative:   "Exodrive changes this program's setting. The program reads it at its next start.",
	SwitchLink:     "Exodrive links the old folder to the copy. The program sees no change.",
	SwitchManual:   "This cache is not in use. Delete it to free the space.",

	Details:         "Details",
	HideDetails:     "Hide details",
	MoveRecords:     "Move records",
	MoveRecordsHelp: "A move back needs the record file and the drive. Keep both.",

	AndMore:                 "and %d more",
	MoveQuestion:            "Move %s?",
	MoveDescription:         "%s moves to %s. Exodrive checks each copy, then frees the space on this Mac.",
	Cancel:                  "Cancel",
	NothingMoved:            "Nothing moved.",
	MoveBackQuestion:        "Move this cache back?",
	MoveBackDescription:     "Exodrive copies %s from %s to its old folder. It checks the copy, then removes the copy on the drive.",
	FromTheDrive:            "the drive",
	DeleteQuestion:          "Delete this cache?",
	DeleteDescription:       "Exodrive deletes %s from this Mac. The data is gone after this step.",
	DeleteDescriptionNoPath: "Exodrive deletes this cache from this Mac. The data is gone after this step.",
	NothingDeleted:          "Nothing deleted.",

	InterruptedTitle:   "The window closed during a move",
	InterruptedBody:    "%s was moving when Exodrive stopped.",
	InterruptedBodyAny: "A cache was moving when Exodrive stopped.",
	InterruptedHint:    "Run a scan. A cache that shows On the drive finished its move. Move again only if it shows On this Mac.",
	NoFolderPathTitle:  "This system names no folder path",
	NoFolderPathBody:   "Exodrive needs the path of the folder.",
	NoFolderPathHint:   "Type the folder path in the field.",
	CheckingCaches:     "Checking this Mac for caches...",
	CheckFailedTitle:   "Exodrive cannot check this Mac",
	CheckPartialTitle:  "Part of the check did not run",
	CheckPartialBody:   "Exodrive could not read every program, so the list may be short.",
	CheckPartialHint:   "Quit the programs below and scan again.",
	FoundCaches:        "Exodrive found %s. %d can move.",
	ChooseFirstTitle:   "Choose a folder first",
	ChooseFirstBody:    "Exodrive needs a checked folder on the external drive.",
	ChooseFirstHint:    "Choose a folder, then try again.",
	MovingStatus:       "Moving %s...",
	StoppedStatus:      "Stopped. %s moved. The other caches stay on this Mac.",
	SomeMovedStatus:    "%s moved. %s did not move.",
	SomeMovedTitleOne:  "1 cache did not move",
	SomeMovedTitleMany: "%d caches did not move",
	SomeMovedBody:      "Each reason sits on its cache row below. Exodrive deleted nothing.",
	SomeMovedHint:      "Open a cache to read the reason, then try again.",
	MovedStatus:        "Moved %s. %s is now free on this Mac.",
	NotInListStatus:    "This cache is not in the list anymore. Run a scan.",
	DeletingStatus:     "Deleting %s...",
	DeletedStatus:      "Deleted %s. %s is now free.",
	RollingBackStatus:  "Moving the cache back to this Mac...",
	RolledBackStatus:   "The cache is back on this Mac. Exodrive removed the record of the move.",
	RetryHint:          "Exodrive deleted nothing. Try again.",
	PickerFailedTitle:  "The folder picker did not open",
	PickerFailedHint:   "Type the folder path in the field, then press Choose again.",

	MoveStoppedTitle:    "The move stopped",
	MoveBackStopped:     "The move back stopped",
	DeleteStopped:       "The delete stopped",
	DriveUnsupported:    "Exodrive cannot use this drive",
	DriveUnsupportedWhy: "The drive uses a format that Exodrive cannot use for caches.",
	DriveUnsupportedFix: "Choose a folder on another drive. Exodrive supports APFS, HFS+, ext2, ext3, ext4, btrfs, zfs, xfs, f2fs, and bcachefs.",
	DriveFull:           "The drive is full",
	DriveFullWhy:        "The drive does not have enough free space for this cache.",
	DriveFullFix:        "Free space on the drive, or move fewer caches at a time.",
	StillRunning:        "A program still uses this cache",
	StillRunningWhy:     "A program that writes to this cache is open.",
	StillRunningFix:     "Quit the program, then try again.",
	AlreadyLink:         "This folder already links elsewhere",
	AlreadyLinkWhy:      "The folder points to a copy that Exodrive did not record.",
	AlreadyLinkFix:      "Run a scan. If the row keeps this message, the cache needs no move.",
	CopyMismatch:        "The copy did not match",
	CopyMismatchWhy:     "The copy on the drive differs from the original.",
	CopyMismatchFix:     "Exodrive deleted nothing. Try again. If it fails again, check the drive.",
	ShellVariable:       "Your shell sets the cache path",
	ShellVariableWhy:    "A variable in your shell overrides the setting that Exodrive changes.",
	ShellVariableFix:    "Remove %s from your shell, then try again.",
	NoRecord:            "No record of this move",
	NoRecordWhy:         "Exodrive cannot find the move that put this cache on the drive.",
	NoRecordFix:         "Run a scan, then choose the cache again.",
	LedgerUnreadable:    "Exodrive cannot read its move records",
	LedgerUnreadableWhy: "The record file is missing or unreadable.",
	LedgerUnreadableFix: "Check that the record file is readable, then scan again.",
	TwoCopies:           "Two copies hold different data",
	TwoCopiesWhy:        "The folder on this Mac and the copy on the drive are not the same.",
	TwoCopiesFix:        "Both copies are safe. Exodrive stops so it cannot pick one for you.",
	FolderGone:          "The folder is gone",
	FolderGoneWhy:       "The folder that Exodrive needs is not there.",
	FolderGoneFix:       "Choose the folder again, then try again.",

	DriveCannotHold:      "This drive cannot hold caches. Choose a folder on an APFS, HFS+, ext2, ext3, ext4, btrfs, zfs, xfs, f2fs, or bcachefs drive.",
	TargetMissingMessage: "This folder does not exist. Choose a folder on the drive.",
	VolumeMissing:        "The external volume %s is not connected.",
	VolumeMissingFix:     "Connect it, then check again.",
	FolderMissing:        "The folder %s does not exist.",
	FolderMissingFix:     "Choose a folder that exists.",
}

var chineseText = text{
	WelcomeTitle:        "迁移缓存只需三步",
	WelcomeStepOne:      "选择外接磁盘上的一个目录。",
	WelcomeStepTwo:      "勾选要迁移的缓存。",
	WelcomeStepThree:    "点「迁移」。副本校验通过后，缓存才释放空间。",
	WelcomeStart:        "选择目录",
	WelcomeSkip:         "跳过",
	Subtitle:            "把开发缓存搬到外接磁盘。",
	CheckingThisMac:     "正在检查这台 Mac",
	CheckingThisMacLine: "正在检查这台 Mac……",
	CheckPromise:        "副本通过校验后，缓存才释放空间。",

	ThisMac:           "这台 Mac",
	ExternalDrive:     "外接磁盘",
	FreeUnknown:       "可用空间未知",
	FreeAmount:        "%s 可用",
	NoCachesFound:     "未发现缓存",
	InCaches:          "缓存占用 %s",
	NoFolderChosen:    "尚未选择目录",
	NotReady:          "不可用",
	FolderFailedCheck: "该目录未通过检查。",
	MovedHere:         "已迁入 %s",
	NoCachesMovedYet:  "还没有迁移缓存",
	NoExternalDrive:   "未连接外接磁盘。",
	DriveConnected:    "已连接 %s。",
	DrivesConnected:   "已连接 %d 块磁盘。",
	DriveChooseFolder: "%s：请选择目录，把缓存搬到这里。",
	DriveMostFree:     "%s 的可用空间最多。另有 %d 块已连接。请选择目录。",
	FolderOnThisMac:   "这个目录在本机磁盘上，搬到这里不会释放空间。",
	FolderPlaceholder: "外接磁盘上的目录",
	Choose:            "选择",

	ChooseAnotherFolder:  "换一个目录",
	ChooseAnotherHelper:  "Exodrive 检查过这个目录，无法使用。",
	ChooseFolderTitle:    "选择缓存目录",
	ChooseFolderHelper:   "缓存会搬到这个目录。请保持磁盘连接。",
	NoCachesInView:       "当前视图没有缓存",
	NoCachesInViewHelper: "点左侧的「全部」或其他分组。",
	NothingCanMove:       "现在没有可迁移的缓存",
	NothingCanMoveHelper: "缓存已在磁盘上，或者无法迁移。",
	SelectCaches:         "选择要迁移的缓存",
	SelectCachesHelper:   "副本通过校验之前，缓存留在本机。",
	MoveTitle:            "迁移 %s",
	MoveHelper:           "%s 将搬到磁盘。每份副本校验通过后才释放空间。",
	ChooseFolderButton:   "选择目录",
	SelectAll:            "全选 %d 个",
	MoveButton:           "迁移 %s",
	CacheOne:             "个缓存",
	CacheMany:            "个缓存",

	Moving:             "正在迁移 %s",
	CheckingCopy:       "正在校验 %s 的副本",
	CacheOfDone:        "第 %d 个，共 %d 个。已完成 %.0f%%。",
	MovingCaches:       "正在迁移缓存",
	Finishing:          "正在完成 %s",
	StoppingAfter:      "本个缓存完成后停止。",
	StopAfterThisCache: "本个完成后停止",

	Caches:            "缓存",
	Selected:          "已选 %d 个",
	Clear:             "清除",
	Rescan:            "重新检查",
	Place:             "位置",
	All:               "全部",
	OnThisMac:         "在本机",
	OnTheDrive:        "在磁盘上",
	Detected:          "已检测",
	AllCaches:         "全部缓存",
	RailLabel:         "%s，%s",
	FamilyOther:       "其他",
	EmptyCheckList:    "Exodrive 检查 npm、pnpm、yarn、cargo、Go、bun、uv、pip 和 Docker。",
	EmptyThisView:     "当前视图没有缓存",
	EmptyThisViewHelp: "点左侧的「全部」或其他分组。",

	Folder:         "目录",
	Copy:           "副本",
	Move:           "迁移",
	MoveBack:       "搬回",
	DeleteCache:    "删除缓存",
	TryAgain:       "重试",
	MoveFailed:     "迁移失败",
	MoveBackFailed: "搬回失败",
	CannotMove:     "无法迁移",
	Unused:         "未使用",
	MoveStopped:    "下面的迁移已停止。读完原因后重试。",
	UnusedNote:     "程序已不再使用这个缓存。",
	SwitchNative:   "Exodrive 会修改这个程序的设置。程序下次启动时读取。",
	SwitchLink:     "Exodrive 把原目录链接到副本。程序不需要改动。",
	SwitchManual:   "这个缓存没被使用。删除它即可释放空间。",

	Details:         "详细信息",
	HideDetails:     "收起详细信息",
	MoveRecords:     "迁移记录",
	MoveRecordsHelp: "搬回需要记录文件和磁盘，请同时保留。",

	AndMore:                 "另有 %d 个",
	MoveQuestion:            "迁移 %s？",
	MoveDescription:         "%s 将搬到 %s。Exodrive 校验每份副本后，再释放本机空间。",
	Cancel:                  "取消",
	NothingMoved:            "没有任何缓存被迁移。",
	MoveBackQuestion:        "把这个缓存搬回本机？",
	MoveBackDescription:     "Exodrive 把 %s 从 %s 复制回原目录。校验通过后，删除磁盘上的副本。",
	FromTheDrive:            "磁盘",
	DeleteQuestion:          "删除这个缓存？",
	DeleteDescription:       "Exodrive 会从本机删除 %s。删除后数据无法恢复。",
	DeleteDescriptionNoPath: "Exodrive 会从本机删除这个缓存。删除后数据无法恢复。",
	NothingDeleted:          "没有删除任何缓存。",

	InterruptedTitle:   "上一次运行在迁移途中结束",
	InterruptedBody:    "Exodrive 停止时正在迁移 %s。",
	InterruptedBodyAny: "Exodrive 停止时有个缓存正在迁移。",
	InterruptedHint:    "重新检查。显示「在磁盘上」的缓存已完成迁移；仍显示「在本机」的，请再迁移一次。",
	NoFolderPathTitle:  "这个系统没有给出目录路径",
	NoFolderPathBody:   "Exodrive 需要目录的路径。",
	NoFolderPathHint:   "在输入框里填写目录路径。",
	CheckingCaches:     "正在检查这台 Mac 上的缓存……",
	CheckFailedTitle:   "Exodrive 无法检查这台 Mac",
	CheckPartialTitle:  "部分检查没有运行",
	CheckPartialBody:   "Exodrive 没能读取所有程序，列表可能不全。",
	CheckPartialHint:   "退出下面列出的程序，然后重新检查。",
	FoundCaches:        "发现 %s，其中 %d 个可以迁移。",
	ChooseFirstTitle:   "请先选择目录",
	ChooseFirstBody:    "Exodrive 需要外接磁盘上已通过检查的目录。",
	ChooseFirstHint:    "选择目录，然后重试。",
	MovingStatus:       "正在迁移 %s……",
	StoppedStatus:      "已停止。已迁移 %s，其余缓存留在本机。",
	SomeMovedStatus:    "已迁移 %s，%s 没有迁移。",
	SomeMovedTitleOne:  "1 个缓存没有迁移",
	SomeMovedTitleMany: "%d 个缓存没有迁移",
	SomeMovedBody:      "每个原因写在下面对应的卡片上。Exodrive 没有删除任何数据。",
	SomeMovedHint:      "点开卡片读完原因，然后重试。",
	MovedStatus:        "已迁移 %s，本机释放 %s。",
	NotInListStatus:    "这个缓存已不在列表里。请重新检查。",
	DeletingStatus:     "正在删除 %s……",
	DeletedStatus:      "已删除 %s，释放 %s。",
	RollingBackStatus:  "正在把这个缓存搬回本机……",
	RolledBackStatus:   "缓存已回到本机。Exodrive 删除了这条迁移记录。",
	RetryHint:          "Exodrive 没有删除任何数据。请重试。",
	PickerFailedTitle:  "目录选择器没有打开",
	PickerFailedHint:   "在输入框里填写目录路径，然后再次点「选择」。",

	MoveStoppedTitle:    "迁移已停止",
	MoveBackStopped:     "搬回已停止",
	DeleteStopped:       "删除已停止",
	DriveUnsupported:    "Exodrive 无法使用这块磁盘",
	DriveUnsupportedWhy: "这块磁盘的格式不能用来存放缓存。",
	DriveUnsupportedFix: "请选另一块磁盘上的目录。Exodrive 支持 APFS、HFS+、ext2、ext3、ext4、btrfs、zfs、xfs、f2fs 和 bcachefs。",
	DriveFull:           "磁盘空间不足",
	DriveFullWhy:        "磁盘没有足够的空间放下这个缓存。",
	DriveFullFix:        "先释放磁盘空间，或少选几个缓存。",
	StillRunning:        "有程序仍在使用这个缓存",
	StillRunningWhy:     "有程序正在向这个缓存写入。",
	StillRunningFix:     "退出该程序，然后重试。",
	AlreadyLink:         "这个目录已经链接到别处",
	AlreadyLinkWhy:      "目录指向一份 Exodrive 没有记录的副本。",
	AlreadyLinkFix:      "重新检查。如果卡片仍然显示这句话，这个缓存不需要迁移。",
	CopyMismatch:        "副本与原文件不一致",
	CopyMismatchWhy:     "磁盘上的副本与原文件不同。",
	CopyMismatchFix:     "Exodrive 没有删除任何数据。请重试。如果仍然失败，检查磁盘。",
	ShellVariable:       "你的 shell 设置了缓存路径",
	ShellVariableWhy:    "shell 里的变量覆盖了 Exodrive 要修改的设置。",
	ShellVariableFix:    "从 shell 里移除 %s，然后重试。",
	NoRecord:            "没有这次迁移的记录",
	NoRecordWhy:         "Exodrive 找不到把这个缓存搬到磁盘的记录。",
	NoRecordFix:         "重新检查，然后再次选择这个缓存。",
	LedgerUnreadable:    "Exodrive 读不到迁移记录",
	LedgerUnreadableWhy: "记录文件缺失或无法读取。",
	LedgerUnreadableFix: "确认记录文件可读，然后重新检查。",
	TwoCopies:           "两份副本的数据不同",
	TwoCopiesWhy:        "本机目录与磁盘副本内容不一致。",
	TwoCopiesFix:        "两份数据都还在。Exodrive 停下来，不替你选择。",
	FolderGone:          "目录不见了",
	FolderGoneWhy:       "Exodrive 需要的目录不在原处。",
	FolderGoneFix:       "重新选择目录，然后重试。",

	DriveCannotHold:      "这块磁盘不能存放缓存。请选 APFS、HFS+、ext2、ext3、ext4、btrfs、zfs、xfs、f2fs 或 bcachefs 磁盘上的目录。",
	TargetMissingMessage: "这个目录不存在。请选磁盘上的目录。",
	VolumeMissing:        "外接卷 %s 未连接。",
	VolumeMissingFix:     "连接后重新检查。",
	FolderMissing:        "目录 %s 不存在。",
	FolderMissingFix:     "请选一个存在的目录。",
}

// languageOf picks the window's language: LangEnvVar first, then the system
// locale, then English. Only a language the window writes in is returned.
func languageOf(cx *core.Scope) lang {
	if want := strings.TrimSpace(os.Getenv(LangEnvVar)); want != "" {
		return langFor(want)
	}
	return langFor(cx.Env().Locale())
}

// lang is a language the window writes in.
type lang string

const (
	langEnglish lang = "en"
	langChinese lang = "zh"
)

// langFor reads a language tag: a tag that starts with zh is Chinese, and any
// other tag is English, because the window writes in these two.
func langFor(tag string) lang {
	tag = strings.ToLower(strings.TrimSpace(tag))
	if strings.HasPrefix(tag, "zh") {
		return langChinese
	}
	return langEnglish
}

// texts returns the words for l.
func texts(l lang) text {
	if l == langChinese {
		return chineseText
	}
	return englishText
}

// counted names a number of caches in the window's language.
func (t text) counted(n int) string {
	one, many := t.CacheOne, t.CacheMany
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}
