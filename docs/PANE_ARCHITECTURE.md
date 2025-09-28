# Binary Tree / Pane Handling Architecture Documentation

This document provides an in-depth analysis of the binary tree and pane handling system in the Dumber browser application, based on thorough examination of the actual codebase.

## Table of Contents

1. [Overall Architecture](#overall-architecture)
2. [Core Data Structures](#core-data-structures)
3. [Component Hierarchy](#component-hierarchy)
4. [Binary Tree Operations](#binary-tree-operations)
5. [Widget Layer Integration](#widget-layer-integration)
6. [Focus Management System](#focus-management-system)
7. [Safety and Validation Systems](#safety-and-validation-systems)
8. [Operation Flow Diagrams](#operation-flow-diagrams)

## Overall Architecture

The pane handling system implements a sophisticated binary tree structure for managing browser panes (similar to tmux/Zellij), with multiple layers of abstraction and safety systems.

```mermaid
graph TB
    subgraph "Application Layer"
        BrowserApp[BrowserApp]
        BrowserPane[BrowserPane]
    end

    subgraph "Workspace Management Layer"
        WorkspaceManager[WorkspaceManager]
        paneNode[paneNode - Binary Tree Node]
        StackedPaneManager[StackedPaneManager]
        FocusStateMachine[FocusStateMachine]
    end

    subgraph "Safety Layer"
        TreeValidator[TreeValidator]
        GeometryValidator[GeometryValidator]
        WidgetTxManager[WidgetTransactionManager]
        ConcurrencyController[ConcurrencyController]
        TreeRebalancer[TreeRebalancer]
        StateTombstoneManager[StateTombstoneManager]
    end

    subgraph "Widget Layer"
        SafeWidget[SafeWidget Wrapper]
        WidgetRegistry[WidgetRegistry]
    end

    subgraph "GTK/WebKit Layer"
        GtkPaned[GtkPaned - Split Container]
        GtkBox[GtkBox - Stack Container]
        WebView[WebKitWebView]
        GTKWindow[GtkWindow]
    end

    BrowserApp --> WorkspaceManager
    WorkspaceManager --> paneNode
    WorkspaceManager --> StackedPaneManager
    WorkspaceManager --> FocusStateMachine
    WorkspaceManager --> TreeValidator
    WorkspaceManager --> GeometryValidator
    paneNode --> SafeWidget
    SafeWidget --> GtkPaned
    SafeWidget --> GtkBox
    SafeWidget --> WebView
    BrowserPane --> WebView
```

## Core Data Structures

### paneNode Structure

The `paneNode` is the fundamental building block of the binary tree:

```mermaid
classDiagram
    class paneNode {
        +BrowserPane pane
        +paneNode parent
        +paneNode left
        +paneNode right
        +SafeWidget container
        +Orientation orientation
        +bool isLeaf
        +bool isPopup
        +WindowType windowType
        +WindowFeatures windowFeatures
        +bool isRelated
        +paneNode parentPane
        +bool autoClose
        +uintptr hoverToken
        +uintptr focusControllerToken
        +bool pendingHoverReattach
        +bool pendingFocusReattach
        +bool isStacked
        +[]*paneNode stackedPanes
        +int activeStackIndex
        +SafeWidget titleBar
        +SafeWidget stackWrapper
    }

    class BrowserPane {
        +WebView webView
        +MessageHandler messageHandler
        +ZoomController zoomController
        +Cleanup()
        +CleanupFromWorkspace()
    }

    class SafeWidget {
        +uintptr ptr
        +string typeInfo
        +bool valid
        +Execute(func(uintptr) error)
        +Invalidate()
        +IsValid() bool
    }

    paneNode --> BrowserPane
    paneNode --> SafeWidget
```

### WorkspaceManager Structure

```mermaid
classDiagram
    class WorkspaceManager {
        +BrowserApp app
        +Window window
        +paneNode root
        +paneNode mainPane
        +map[WebView]paneNode viewToNode
        +map[WebView]time.Time lastSplitMsg
        +map[WebView]time.Time lastExitMsg
        +bool paneModeActive
        +int32 splitting
        +bool cssInitialized
        +func createWebViewFn
        +func createPaneFn
        +WebView paneModeSource
        +time.Time lastPaneModeEntry
        +sync.Mutex paneMutex
        +StackedPaneManager stackedPaneManager
        +FocusStateMachine focusStateMachine
        +sync.Mutex widgetMutex
        +WidgetRegistry widgetRegistry
        +TreeValidator treeValidator
        +WidgetTransactionManager widgetTxManager
        +ConcurrencyController concurrencyController
        +TreeRebalancer treeRebalancer
        +GeometryValidator geometryValidator
        +StackLifecycleManager stackLifecycleManager
        +StateTombstoneManager stateTombstoneManager
    }
```

## Component Hierarchy

The system follows a clear hierarchical structure:

```mermaid
graph TD
    subgraph "Tree Structure"
        Root[Root paneNode]
        Split1[Split paneNode<br/>GtkPaned Horizontal]
        Split2[Split paneNode<br/>GtkPaned Vertical]
        Stack1[Stack paneNode<br/>GtkBox Container]
        Leaf1[Leaf paneNode<br/>WebView 1]
        Leaf2[Leaf paneNode<br/>WebView 2]
        Leaf3[Leaf paneNode<br/>WebView 3]
        Leaf4[Leaf paneNode<br/>WebView 4]
        Leaf5[Leaf paneNode<br/>WebView 5]
    end

    Root --> Split1
    Root --> Leaf1
    Split1 --> Split2
    Split1 --> Stack1
    Split2 --> Leaf2
    Split2 --> Leaf3
    Stack1 --> Leaf4
    Stack1 --> Leaf5

    style Root fill:#e1f5fe
    style Split1 fill:#f3e5f5
    style Split2 fill:#f3e5f5
    style Stack1 fill:#e8f5e8
    style Leaf1 fill:#fff3e0
    style Leaf2 fill:#fff3e0
    style Leaf3 fill:#fff3e0
    style Leaf4 fill:#fff3e0
    style Leaf5 fill:#fff3e0
```

## Binary Tree Operations

### Split Operation Flow

```mermaid
sequenceDiagram
    participant User
    participant WM as WorkspaceManager
    participant BP as BulletproofOps
    participant TV as TreeValidator
    participant GV as GeometryValidator
    participant GTK as GTK Layer

    User->>WM: Request split (target, direction)
    WM->>BP: BulletproofSplitNode()
    BP->>TV: ValidateTree(before_split)
    TV-->>BP: Validation result
    BP->>GV: ValidateSplit(target, direction)
    GV-->>BP: Geometry validation

    alt Validation passes
        BP->>WM: splitNode(target, direction)
        WM->>GTK: Create GtkPaned
        GTK-->>WM: Paned widget
        WM->>GTK: Reparent existing widget
        WM->>GTK: Add new WebView
        WM->>WM: Update tree structure
        WM-->>BP: New pane node
        BP->>TV: ValidateTree(after_split)
        BP-->>User: Success + new pane
    else Validation fails
        BP-->>User: Error message
    end
```

### Close Operation Flow

```mermaid
sequenceDiagram
    participant User
    participant WM as WorkspaceManager
    participant BP as BulletproofOps
    participant STM as StateTombstoneManager
    participant TV as TreeValidator
    participant TR as TreeRebalancer
    participant GTK as GTK Layer

    User->>WM: Request close (target)
    WM->>BP: BulletproofClosePane()
    BP->>STM: CaptureState("close")
    STM-->>BP: State tombstone
    BP->>TV: ValidateTree(before_close)
    TV-->>BP: Validation result

    alt Has sibling
        BP->>WM: closePane(target)
        WM->>WM: Find sibling/replacement node
        WM->>GTK: Unparent target widget
        WM->>GTK: Promote sibling to new position
        WM->>WM: Update tree structure
        WM-->>BP: Return promoted node
        BP->>TV: ValidateTree(after_close)
        BP->>TR: RebalanceAfterClose(closed, promoted)
        TR->>TR: Execute promotion transaction
        TR->>GTK: WidgetResetSizeRequest(promoted)
        TR->>GTK: Reattach with proper expansion
        TR->>GTK: Queue allocation on ancestors
        TR->>TR: Validate geometry
        TR-->>BP: Promotion completed
        BP-->>User: Success
    else Last pane
        WM->>GTK: QuitMainLoop()
        WM-->>BP: Return nil (app exiting)
    else Error occurs
        BP->>STM: RestoreState(tombstone)
        BP-->>User: Error + rollback
    end
```

### Stack Operation

```mermaid
sequenceDiagram
    participant User
    participant WM as WorkspaceManager
    participant SPM as StackedPaneManager
    participant GTK as GTK Layer

    User->>WM: Request stack (target)
    WM->>SPM: StackPane(target)
    SPM->>GTK: Create GtkBox container
    GTK-->>SPM: Box widget
    SPM->>GTK: Create title bar
    SPM->>GTK: Add target to stack
    SPM->>GTK: Create new WebView
    SPM->>GTK: Add new pane to stack
    SPM->>SPM: Update stack navigation
    SPM-->>WM: New stacked node
    WM-->>User: Success
```

## Widget Layer Integration

### SafeWidget Architecture

```mermaid
graph TB
    subgraph "SafeWidget System"
        SW[SafeWidget]
        WR[WidgetRegistry]
        WT[WidgetTransaction]
    end

    subgraph "GTK Widgets"
        GP[GtkPaned]
        GB[GtkBox]
        WV[WebKitWebView]
        GL[GtkLabel]
    end

    SW --> WR
    SW --> WT
    SW --> GP
    SW --> GB
    SW --> WV
    SW --> GL

    WR -.-> |"Track lifetimes"| GP
    WR -.-> |"Track lifetimes"| GB
    WR -.-> |"Track lifetimes"| WV

    WT -.-> |"Atomic operations"| GP
    WT -.-> |"Atomic operations"| GB
    WT -.-> |"Atomic operations"| WV
```

### Widget Lifecycle Management

```mermaid
stateDiagram-v2
    [*] --> Created
    Created --> Registered: Register with WidgetRegistry
    Registered --> Realized: WidgetRealizeInContainer()
    Realized --> Parented: Add to parent widget
    Parented --> Visible: WidgetShow()
    Visible --> Processing: Normal operation
    Processing --> Reparenting: During splits/closes
    Reparenting --> Unparented: WidgetUnparent()
    Unparented --> Parented: New parent assigned
    Processing --> Invalidated: Widget destroyed
    Invalidated --> [*]

    Reparenting --> ValidationFailed: Widget becomes invalid
    ValidationFailed --> Invalidated
```

## Focus Management System

### Focus State Machine

```mermaid
stateDiagram-v2
    [*] --> Initializing
    Initializing --> Idle: System ready
    Idle --> Transitioning: Focus request received
    Transitioning --> Focused: Focus applied successfully
    Focused --> Idle: Focus settled
    Transitioning --> Reconciling: Focus conflict detected
    Reconciling --> Focused: Conflict resolved
    Reconciling --> Idle: Fallback to previous state

    note right of Transitioning: Priority queue processes requests
    note right of Reconciling: CSS class synchronization
    note right of Focused: GTK focus + CSS classes applied
```

### Focus Request Processing

```mermaid
graph TB
    subgraph "Focus Request Flow"
        FR[Focus Request]
        DD[Request Deduplicator]
        PQ[Priority Queue]
        FSM[Focus State Machine]
        GC[GTK Controllers]
        CSS[CSS Classes]
    end

    FR --> DD
    DD --> |"Not duplicate"| PQ
    DD --> |"Duplicate"| Discard[Discard]
    PQ --> FSM
    FSM --> GC
    FSM --> CSS

    subgraph "Priority Levels"
        PU[Urgent: 100]
        PH[High: 90]
        PN[Normal: 50]
        PL[Low: 10]
    end

    PQ --> PU
    PQ --> PH
    PQ --> PN
    PQ --> PL
```

## Safety and Validation Systems

### Tree Validation

```mermaid
graph TB
    subgraph "Tree Validation System"
        TV[TreeValidator]
        VS[Validation Suite]

        subgraph "Validation Checks"
            PC[Parent-Child Consistency]
            RC[Root Node Validation]
            LC[Leaf Node Validation]
            SC[Stack Consistency]
            WC[Widget Consistency]
        end
    end

    TV --> VS
    VS --> PC
    VS --> RC
    VS --> LC
    VS --> SC
    VS --> WC

    PC -.-> |"Bidirectional links"| TreeStructure[Tree Structure]
    RC -.-> |"Single root"| TreeStructure
    LC -.-> |"Valid panes"| TreeStructure
    SC -.-> |"Stack integrity"| TreeStructure
    WC -.-> |"Widget pointers"| TreeStructure
```

### Bulletproof Operation Wrapper

```mermaid
sequenceDiagram
    participant Client
    participant BP as BulletproofOps
    participant STM as StateTombstone
    participant GV as GeometryValidator
    participant TV as TreeValidator
    participant CC as ConcurrencyController
    participant TR as TreeRebalancer
    participant Core as CoreOperation

    Client->>BP: Operation request
    BP->>STM: Capture state tombstone
    BP->>GV: Validate geometry constraints
    BP->>TV: Validate tree before operation

    alt On GTK main thread
        BP->>Core: Execute directly
    else Off main thread
        BP->>CC: Submit to concurrency controller
        CC->>Core: Execute on main thread
    end

    Core-->>BP: Operation result

    alt Success
        BP->>TV: Validate tree after operation
        BP->>TR: Rebalance if needed
        BP-->>Client: Success
    else Failure
        BP->>STM: Restore state from tombstone
        BP-->>Client: Error + rollback
    end
```

## Operation Flow Diagrams

### Complete Split Operation

```mermaid
flowchart TD
    Start([Split Request]) --> V1{Tree Valid?}
    V1 -->|No| Error1[Return Error]
    V1 -->|Yes| V2{Geometry Valid?}
    V2 -->|No| Error2[Return Error]
    V2 -->|Yes| Capture[Capture State Tombstone]

    Capture --> Thread{On GTK Thread?}
    Thread -->|Yes| Direct[Execute Directly]
    Thread -->|No| Queue[Submit to Concurrency Controller]
    Queue --> Execute[Execute on Main Thread]
    Direct --> Execute

    Execute --> CreatePaned[Create GtkPaned Widget]
    CreatePaned --> CreateWebView[Create New WebView]
    CreateWebView --> DetachControllers[Safely Detach GTK Controllers]
    DetachControllers --> UnparentTarget[Unparent Target Widget]
    UnparentTarget --> SetupPaned[Configure Paned Properties]
    SetupPaned --> AddChildren[Add Both Widgets to Paned]
    AddChildren --> UpdateTree[Update Tree Structure]
    UpdateTree --> AttachPaned[Attach Paned to Parent/Window]
    AttachPaned --> ReattachControllers[Reattach GTK Controllers]
    ReattachControllers --> ShowWidgets[Show All Widgets]
    ShowWidgets --> UpdateCSS[Update CSS Classes]
    UpdateCSS --> SetFocus[Set Focus to New Pane]
    SetFocus --> Validate[Validate Tree Structure]
    Validate --> Rebalance[Tree Rebalancing Check]
    Rebalance --> Success([Return New Pane])

    Execute -->|Error| Rollback[Restore from Tombstone]
    Rollback --> Error3[Return Error]

    style Start fill:#e1f5fe
    style Success fill:#e8f5e8
    style Error1 fill:#ffebee
    style Error2 fill:#ffebee
    style Error3 fill:#ffebee
```

### Complete Close Operation

```mermaid
flowchart TD
    Start([Close Request]) --> Stack{In Stack?}
    Stack -->|Yes| StackClose[Use Stack Lifecycle Manager]
    Stack -->|No| ValidateTree[Validate Tree Structure]

    ValidateTree --> Capture[Capture State Tombstone]
    Capture --> CheckLast{Last Pane?}

    CheckLast -->|Yes| QuitApp[Quit Application - Return nil]
    CheckLast -->|No| CheckRoot{Is Root?}

    CheckRoot -->|Yes| FindReplacement[Find Replacement Root]
    CheckRoot -->|No| FindSibling[Find Sibling Node]

    FindReplacement --> HasReplacement{Has Replacement?}
    HasReplacement -->|No| QuitApp
    HasReplacement -->|Yes| PromoteRoot[Promote Replacement to Root]

    FindSibling --> ValidateWidgets[Validate Widget State]
    ValidateWidgets --> CheckGrand{Has Grandparent?}

    CheckGrand -->|Yes| PromoteToParent[Promote Sibling to Parent Position]
    CheckGrand -->|No| PromoteToRoot

    PromoteRoot --> TrackPromoted[Track Promoted Node]
    PromoteToParent --> TrackPromoted

    TrackPromoted --> UnparentFromCurrent[Unparent from Current Parent]
    UnparentFromCurrent --> AttachToNew[Attach to New Parent/Window]
    AttachToNew --> UpdateTreeRefs[Update Tree References]
    UpdateTreeRefs --> CleanupOld[Cleanup Old Parent Node]
    CleanupOld --> FindFocus[Find Focus Target]
    FindFocus --> SetFocus[Set Focus]
    SetFocus --> CleanupPane[Cleanup Closed Pane]
    CleanupPane --> DestroyWebView[Destroy WebView]
    DestroyWebView --> UpdateMain[Update Main Pane Reference]
    UpdateMain --> ReturnPromoted[Return Promoted Node]
    ReturnPromoted --> PromotionTx[Execute Promotion Transaction]
    PromotionTx --> ResetSize[WidgetResetSizeRequest]
    ResetSize --> ReattachWidget[Reattach with Expansion]
    ReattachWidget --> QueueAllocation[Queue Allocation on Ancestors]
    QueueAllocation --> ValidateGeometry[Validate Final Geometry]
    ValidateGeometry --> Success([Success with Promoted Node])

    StackClose --> Success
    QuitApp --> SuccessNil([Success - App Exiting])

    style Start fill:#e1f5fe
    style Success fill:#e8f5e8
    style SuccessNil fill:#fff3e0
    style TrackPromoted fill:#e3f2fd
    style PromotionTx fill:#f3e5f5
```

## CSS Class Management

The system uses CSS classes for visual feedback and state indication:

```mermaid
graph TB
    subgraph "CSS Class System"
        Base[.workspace-pane]
        Multi[.workspace-multi-pane]
        Active[.workspace-pane-active]
        Outline[.workspace-pane-active-outline]
        Stack[.stacked-pane-container]
    end

    subgraph "Application Logic"
        Single[Single Pane] --> Base
        Multiple[Multiple Panes] --> Base
        Multiple --> Multi
        Focused[Focused Pane] --> Active
        Focused --> Outline
        StackContainer[Stack Container] --> Stack
    end

    subgraph "Visual Effects"
        Base --> BasicStyling[Basic pane styling]
        Multi --> BorderStyling[Pane borders in multi-pane mode]
        Active --> FocusHighlight[Focus highlight]
        Outline --> FocusOutline[Focus outline border]
        Stack --> StackStyling[Stack container styling]
    end
```

## Enhanced Promotion System

The system now includes a sophisticated promotion transaction system that ensures proper widget layout after pane closures:

### Promotion Transaction Flow

```mermaid
sequenceDiagram
    participant BP as BulletproofOps
    participant WM as WorkspaceManager
    participant TR as TreeRebalancer
    participant WT as WidgetTransaction
    participant GTK as GTK Layer

    BP->>WM: closePane(node)
    WM->>WM: Identify promoted node
    WM->>GTK: Update tree structure
    WM-->>BP: Return promoted node

    BP->>TR: RebalanceAfterClose(closed, promoted)
    TR->>WT: Begin promotion transaction

    Note over TR,WT: Widget size constraint reset
    TR->>WT: Add WidgetResetSizeRequest operation

    Note over TR,WT: Widget reattachment with expansion
    TR->>WT: Add reattachment operation

    Note over TR,WT: Ancestor allocation propagation
    TR->>WT: Add allocation queue operations

    Note over TR,WT: Geometry validation
    TR->>WT: Add validation operation

    WT->>GTK: Execute all operations atomically
    GTK-->>WT: Operations completed
    WT->>WT: Commit transaction
    WT-->>TR: Transaction success
    TR-->>BP: Promotion completed
```

### Key Promotion Features

- **Size Constraint Reset**: `WidgetResetSizeRequest()` clears stale size constraints
- **Proper Expansion**: Ensures promoted widgets can expand to fill available space
- **Ancestor Propagation**: Queues allocation updates up the widget hierarchy
- **Geometry Validation**: Validates final widget bounds after promotion
- **Transaction Safety**: All operations are atomic with rollback capability

## Key Files and Their Responsibilities

| File | Purpose | Key Components |
|------|---------|----------------|
| `workspace_types.go:8` | Core data structures | `paneNode`, CSS constants |
| `workspace_manager.go:18` | Main coordinator | `WorkspaceManager`, initialization |
| `workspace_pane_ops.go:707` | Tree operations | Split, close, stack operations with promoted node tracking |
| `workspace_tree_rebalancer.go:92` | Tree balancing | Rebalancing algorithms, promotion transactions |
| `workspace_bulletproof_operations.go:158` | Safety wrapper | Bulletproof operation wrappers with promotion handling |
| `workspace_concurrency.go:454` | Async operations | Concurrency controller with promoted node results |
| `focus_state_machine.go:186` | Focus management | State machine, priority queue |
| `workspace_widgets_cgo.go:633` | GTK integration | Widget operations, WidgetResetSizeRequest |

## Performance Characteristics

- **Tree Height**: O(log n) for balanced trees, O(n) worst case
- **Split Operation**: O(log n) average case with rebalancing
- **Close Operation**: O(1) tree updates + O(k) widget operations where k = ancestor depth
- **Focus Changes**: O(1) with priority queue and deduplication
- **Widget Operations**: Atomic transactions prevent partial state
- **Promotion Transactions**: Batched GTK operations minimize UI thread overhead
- **Memory Usage**: Each pane node ~200 bytes + GTK widget overhead

### Recent Performance Improvements

- **Promoted Node Tracking**: Eliminates redundant tree traversals during rebalancing
- **Batched Widget Operations**: Ancestor allocation updates processed in single transaction
- **Size Constraint Management**: Explicit constraint reset prevents GTK layout conflicts
- **Async Consistency**: Identical behavior between sync and async operation paths

## Thread Safety

The system implements comprehensive thread safety:

- **Widget Operations**: All GTK operations marshaled to main thread
- **Focus Management**: Queue-based serialization with priority
- **State Changes**: Mutex protection for critical sections
- **Tree Modifications**: Atomic operations with rollback capability

This architecture provides a robust, efficient, and safe pane management system that can handle complex window layouts while maintaining excellent performance and user experience.