// PROTOTYPE — throwaway. Resolves wayfinder ticket #15 (CmdWarden repo issue #15).
// Three structurally different Approval Gate layouts, switchable by clicking
// 1/2/3 in the top-right pill (keys 1/2/3 also work if keyboard focus grabs).
// Hardcoded sample data only — no real Session Agent/IPC wiring.
//
// Run:  qs -p docs/prototypes/approval-gate
// Quit: click "Quit", right-click anywhere, or Escape
//
// Single PanelWindow / single Wayland surface for the whole prototype —
// an earlier version used one PanelWindow per variant plus a separate
// switcher window, all on WlrLayer.Overlay. The full-screen transparent
// variant windows sat above the switcher in stacking order and silently
// swallowed clicks meant for it (transparent still accepts pointer input
// unless a window explicitly narrows its input region). One window avoids
// that whole class of cross-surface z-order bug — ordinary QML sibling
// stacking (later-declared paints on top) is enough.
//
// Standalone Quickshell instance, NOT an omarchy-shell plugin — deliberately
// avoids importing qs.Commons / qs.Ui (omarchy-shell's own internal theme
// singletons, only resolvable inside its own config tree). Uses the same
// Quickshell.Wayland PanelWindow + WlrLayershell.layer: Overlay pattern
// Omarchy's own first-party polkit dialog uses (see
// /usr/share/omarchy/shell/plugins/polkit/PolkitAgent.qml).

import QtQuick
import Quickshell
import Quickshell.Wayland

ShellRoot {
  id: root

  property int variant: 1
  property var variantNames: ({ 1: "Compact Toast", 2: "Center Modal", 3: "Queue Panel" })

  // ---- Hardcoded sample data (this is what the Session Agent would supply for real) ----
  property string identityKey: "mise:claude"
  property string tool: "gh"
  property string commandLine: "gh repo view BasantPandey/CmdWarden --json defaultBranchRef"
  property string commandClass: "read" // read | write | secret-reveal | unknown
  property string policyLevel: "Read"

  property var pendingQueue: [
    { identityKey: "mise:claude", tool: "gh", commandLine: "gh pr create --title \"fix: retry logic\" --body \"...\"", commandClass: "write" },
    { identityKey: "mise:codex",  tool: "gh", commandLine: "gh secret set DEPLOY_TOKEN",                              commandClass: "secret-reveal" },
    { identityKey: "mise:claude", tool: "gh", commandLine: "gh repo view",                                            commandClass: "read" }
  ]

  function classColor(cls) {
    if (cls === "read") return "#2e7d32"
    if (cls === "write") return "#f9a825"
    if (cls === "secret-reveal") return "#c62828"
    return "#616161"
  }

  function handleKeys(event) {
    if (event.key === Qt.Key_1) { root.variant = 1; event.accepted = true }
    else if (event.key === Qt.Key_2) { root.variant = 2; event.accepted = true }
    else if (event.key === Qt.Key_3) { root.variant = 3; event.accepted = true }
    else if (event.key === Qt.Key_Escape) { Qt.quit() }
  }

  PanelWindow {
    id: win
    anchors { top: true; bottom: true; left: true; right: true }
    color: "transparent"
    WlrLayershell.namespace: "cmdwarden-approval-prototype"
    WlrLayershell.layer: WlrLayer.Overlay
    WlrLayershell.keyboardFocus: WlrKeyboardFocus.Exclusive
    exclusionMode: ExclusionMode.Ignore

    // Dimmed backdrop only for the Center Modal variant.
    Rectangle {
      anchors.fill: parent
      color: "#000000"
      opacity: root.variant === 2 ? 0.45 : 0
    }

    // Background click/key catcher — declared first so cards and the
    // switcher (declared after it) paint on top and get first crack at clicks.
    Item {
      anchors.fill: parent
      focus: true
      Keys.priority: Keys.BeforeItem
      Keys.onPressed: root.handleKeys
      MouseArea { anchors.fill: parent; acceptedButtons: Qt.RightButton; onClicked: Qt.quit() }
    }

    // ---- Variant 1: compact card, centered, no backdrop ----
    Rectangle {
      visible: root.variant === 1
      width: 380
      height: colA.implicitHeight + 24
      anchors.centerIn: parent
      radius: 10
      color: "#20232a"
      border.color: "#3a3f4b"
      border.width: 1

      Column {
        id: colA
        anchors.fill: parent
        anchors.margins: 12
        spacing: 6

        Row {
          spacing: 8
          Rectangle { width: 8; height: 8; radius: 4; color: root.classColor(root.commandClass); anchors.verticalCenter: parent.verticalCenter }
          Text { text: "CmdWarden — " + root.tool; color: "white"; font.bold: true; font.pixelSize: 14 }
        }
        Text { text: root.identityKey + "  ·  policy: " + root.policyLevel; color: "#9aa0ab"; font.pixelSize: 11 }
        Text { text: root.commandLine; color: "#d6d9e0"; font.family: "monospace"; font.pixelSize: 11; wrapMode: Text.Wrap; width: parent.width }

        Row {
          spacing: 8
          Repeater {
            model: ["Deny", "Allow Once", "Allow Session"]
            delegate: Rectangle {
              width: 108; height: 28; radius: 6
              color: index === 0 ? "#4a2020" : (index === 1 ? "#2a2a2a" : "#204a2e")
              border.color: "#555"; border.width: 1
              Text { anchors.centerIn: parent; text: modelData; color: "white"; font.pixelSize: 11 }
              MouseArea { anchors.fill: parent; onClicked: Qt.quit() }
            }
          }
        }
      }
    }

    // ---- Variant 2: centered, focus-grabbing modal (mirrors the polkit dialog's own weight) ----
    Rectangle {
      visible: root.variant === 2
      width: 460
      height: colB.implicitHeight + 40
      anchors.centerIn: parent
      radius: 14
      color: "#181a20"
      border.color: root.classColor(root.commandClass)
      border.width: 2

      Column {
        id: colB
        anchors.fill: parent
        anchors.margins: 20
        spacing: 12

        Text { text: "Approval needed"; color: "white"; font.bold: true; font.pixelSize: 18 }

        Row {
          spacing: 10
          Rectangle {
            width: 96; height: 22; radius: 4; color: root.classColor(root.commandClass)
            Text { anchors.centerIn: parent; text: root.commandClass; color: "white"; font.pixelSize: 10; font.bold: true }
          }
          Text { text: "Launcher: " + root.identityKey; color: "#c7cbd4"; font.pixelSize: 13 }
        }

        Text { text: "Policy level: " + root.policyLevel + " (does not auto-allow " + root.commandClass + ")"; color: "#9aa0ab"; font.pixelSize: 12 }

        Rectangle { width: parent.width; height: 1; color: "#333" }

        Text { text: root.commandLine; color: "#e5e7eb"; font.family: "monospace"; font.pixelSize: 13; wrapMode: Text.Wrap; width: parent.width }

        Row {
          spacing: 10
          width: parent.width
          Repeater {
            model: [
              { l: "Deny", k: "D", c: "#4a2020" },
              { l: "Approve Once", k: "O", c: "#2a2a2a" },
              { l: "Allow for Session", k: "S", c: "#204a2e" }
            ]
            delegate: Rectangle {
              width: 138; height: 36; radius: 8
              color: modelData.c
              border.color: "#666"; border.width: 1
              Text { anchors.centerIn: parent; text: "[" + modelData.k + "] " + modelData.l; color: "white"; font.pixelSize: 12 }
              MouseArea { anchors.fill: parent; onClicked: Qt.quit() }
            }
          }
        }
      }
    }

    // ---- Variant 3: right-edge panel — a QUEUE of pending approvals, not just one ----
    Rectangle {
      visible: root.variant === 3
      anchors { top: parent.top; bottom: parent.bottom; right: parent.right }
      width: 340
      color: "#15171c"

      Column {
        anchors.fill: parent
        anchors.margins: 12
        spacing: 10

        Text { text: "Pending approvals (" + root.pendingQueue.length + ")"; color: "white"; font.bold: true; font.pixelSize: 15 }

        Repeater {
          model: root.pendingQueue
          delegate: Rectangle {
            width: parent.width
            implicitHeight: qCol.implicitHeight + 20
            radius: 8
            color: "#20232a"
            border.color: root.classColor(modelData.commandClass); border.width: 1

            Column {
              id: qCol
              anchors.fill: parent
              anchors.margins: 10
              spacing: 4
              Text { text: modelData.identityKey + " -> " + modelData.tool; color: "#c7cbd4"; font.pixelSize: 11 }
              Text { text: modelData.commandLine; color: "#e5e7eb"; font.family: "monospace"; font.pixelSize: 10; wrapMode: Text.Wrap; width: parent.width }
              Row {
                spacing: 6
                Repeater {
                  model: ["Deny", "Once", "Session"]
                  delegate: Rectangle {
                    width: 68; height: 22; radius: 5; color: "#2a2a2a"; border.color: "#555"; border.width: 1
                    Text { anchors.centerIn: parent; text: modelData; color: "white"; font.pixelSize: 9 }
                    MouseArea { anchors.fill: parent; onClicked: Qt.quit() }
                  }
                }
              }
            }
          }
        }
      }
    }

    // ---- Switcher pill — declared last so it always paints on top of any variant. ----
    Rectangle {
      anchors { top: parent.top; right: parent.right }
      anchors.margins: 12
      width: switcherRow.implicitWidth + 24
      height: switcherRow.implicitHeight + 14
      radius: 6
      color: "#1a1a1a"

      Row {
        id: switcherRow
        anchors.centerIn: parent
        spacing: 10

        Text {
          anchors.verticalCenter: parent.verticalCenter
          text: "Click to switch:"
          color: "white"
          font.pixelSize: 12
        }

        Repeater {
          model: [1, 2, 3]
          delegate: Rectangle {
            width: 26; height: 22; radius: 5
            anchors.verticalCenter: parent.verticalCenter
            color: root.variant === modelData ? "#3a6fd6" : "#333"
            border.color: "#666"; border.width: 1
            Text { anchors.centerIn: parent; text: modelData; color: "white"; font.pixelSize: 12 }
            MouseArea { anchors.fill: parent; onClicked: root.variant = modelData }
          }
        }

        Text {
          anchors.verticalCenter: parent.verticalCenter
          text: "(" + root.variantNames[root.variant] + ")"
          color: "#9aa0ab"
          font.pixelSize: 12
        }

        Rectangle {
          width: 50; height: 22; radius: 5
          anchors.verticalCenter: parent.verticalCenter
          color: "#4a2020"
          border.color: "#666"; border.width: 1
          Text { anchors.centerIn: parent; text: "Quit"; color: "white"; font.pixelSize: 11 }
          MouseArea { anchors.fill: parent; onClicked: Qt.quit() }
        }
      }
    }
  }
}
