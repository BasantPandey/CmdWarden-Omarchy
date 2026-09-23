// CmdWarden-Omarchy Approval Gate — real Session Agent integration.
//
// Derived from Variant 2 (Center Modal) of the throwaway prototype at
// docs/prototypes/approval-gate/shell.qml (wayfinder ticket #15 /
// CmdWarden-Omarchy ticket #6), with variant switching and hardcoded sample
// data removed: every field below comes from an environment variable the
// agent sets before spawning this instance (Quickshell.env), and each
// button submits its answer back to the agent via `cw gate respond` — a
// plain argv Process, run to completion — before the window closes.
//
// Standalone Quickshell instance, NOT an omarchy-shell plugin — see the
// prototype's own header comment for why (avoids coupling to
// omarchy-shell's internal theme singletons and plugin registry). Single
// PanelWindow / single Wayland surface, per the multi-window bug the
// prototype's header also documents.
//
// Run: qs -p <this file's directory>   (the agent does this for you)

import QtQuick
import Quickshell
import Quickshell.Io
import Quickshell.Wayland

ShellRoot {
  id: root

  property string requestId: Quickshell.env("CMDWARDEN_GATE_REQUEST_ID") || ""
  property string identityKey: Quickshell.env("CMDWARDEN_GATE_IDENTITY") || "(unknown)"
  property string tool: Quickshell.env("CMDWARDEN_GATE_TOOL") || "(unknown)"
  property string commandLine: Quickshell.env("CMDWARDEN_GATE_COMMAND") || "(unknown)"
  property string commandClass: Quickshell.env("CMDWARDEN_GATE_CLASS") || "unknown"
  property string policyLevel: Quickshell.env("CMDWARDEN_GATE_POLICY_LEVEL") || "(unknown)"
  property bool offerAllowSession: Quickshell.env("CMDWARDEN_GATE_ALLOW_SESSION") === "1"
  property string cwPath: Quickshell.env("CMDWARDEN_CW_PATH") || "cw"

  property bool responded: false

  function classColor(cls) {
    if (cls === "read") return "#2e7d32"
    if (cls === "write") return "#f9a825"
    if (cls === "secret-reveal") return "#c62828"
    return "#616161"
  }

  function classLabel(cls) {
    if (cls === "read") return "Read"
    if (cls === "write") return "Write"
    if (cls === "secret-reveal") return "Secret Reveal"
    return cls
  }

  function iconForLauncher(identityKey) {
    var parts = identityKey.split(":")
    var launcherTool = parts.length > 1 ? parts[1] : parts[0]
    if (launcherTool === "claude") return "icons/claudecode.svg"
    if (launcherTool === "gemini") return "icons/gemini.svg"
    if (launcherTool === "codex") return "icons/openai.svg"
    return "icons/unknown.svg"
  }

  function iconForTool(tool) {
    if (tool === "gh") return "icons/github.svg"
    if (tool === "git") return "icons/git.svg"
    if (tool === "docker") return "icons/docker.svg"
    if (tool === "az") return "icons/azure.svg"
    return "icons/unknown.svg"
  }

  // Submits the decision then quits — onExited (not onClicked) is what
  // actually closes the window, so a click can never close the surface
  // without the agent having heard back first.
  function respond(decision) {
    if (root.responded) return
    root.responded = true
    respondProc.command = [root.cwPath, "gate", "respond", "--request", root.requestId, "--decision", decision]
    respondProc.running = true
  }

  Process {
    id: respondProc
    running: false
    onExited: Qt.quit()
  }

  PanelWindow {
    id: win
    anchors { top: true; bottom: true; left: true; right: true }
    color: "transparent"
    WlrLayershell.namespace: "cmdwarden-approval-gate"
    WlrLayershell.layer: WlrLayer.Overlay
    WlrLayershell.keyboardFocus: WlrKeyboardFocus.Exclusive
    exclusionMode: ExclusionMode.Ignore

    Rectangle {
      anchors.fill: parent
      color: "#000000"
      opacity: 0.45
    }

    // Background click/key catcher — declared first so the card (declared
    // after it) paints on top and gets first crack at clicks. Right-click
    // or Escape denies rather than silently dismissing: an approval gate
    // that can be bypassed by clicking away isn't a gate.
    Item {
      anchors.fill: parent
      focus: true
      Keys.priority: Keys.BeforeItem
      Keys.onPressed: function(event) {
        if (event.key === Qt.Key_D) { root.respond("deny"); event.accepted = true }
        else if (event.key === Qt.Key_O) { root.respond("allow-once"); event.accepted = true }
        else if (event.key === Qt.Key_S && root.offerAllowSession) { root.respond("allow-session"); event.accepted = true }
        else if (event.key === Qt.Key_Escape) { root.respond("deny"); event.accepted = true }
      }
      MouseArea { anchors.fill: parent; acceptedButtons: Qt.RightButton; onClicked: root.respond("deny") }
    }

    Rectangle {
      width: 580
      height: col.implicitHeight + 48
      anchors.centerIn: parent
      radius: 16
      color: "#181a20"
      border.color: root.classColor(root.commandClass)
      border.width: 2

      Column {
        id: col
        anchors.fill: parent
        anchors.margins: 24
        spacing: 16

        Text { text: "Approval needed"; color: "white"; font.bold: true; font.pixelSize: 24 }

        Row {
          spacing: 12
          Rectangle {
            width: 132; height: 28; radius: 5; color: root.classColor(root.commandClass)
            Text { anchors.centerIn: parent; text: root.classLabel(root.commandClass); color: "white"; font.pixelSize: 13; font.bold: true }
          }
          Row {
            spacing: 10
            Image {
              source: root.iconForLauncher(root.identityKey)
              width: 24; height: 24
              sourceSize.width: 24; sourceSize.height: 24
              fillMode: Image.PreserveAspectFit
              anchors.verticalCenter: parent.verticalCenter
            }
            Text { text: "Launcher: " + root.identityKey; color: "#c7cbd4"; font.pixelSize: 16; anchors.verticalCenter: parent.verticalCenter }
          }
        }

        Row {
          spacing: 10
          width: parent.width
          Image {
            source: root.iconForTool(root.tool)
            width: 20; height: 20
            sourceSize.width: 20; sourceSize.height: 20
            fillMode: Image.PreserveAspectFit
            anchors.verticalCenter: parent.verticalCenter
          }
          Text {
            text: "Tool: " + root.tool + "  ·  Policy level: " + root.policyLevel + " (does not auto-allow " + root.commandClass + ")"
            color: "#9aa0ab"; font.pixelSize: 14; wrapMode: Text.Wrap; width: col.width - 30
            anchors.verticalCenter: parent.verticalCenter
          }
        }

        Rectangle { width: parent.width; height: 1; color: "#333" }

        Text { text: root.commandLine; color: "#e5e7eb"; font.family: "monospace"; font.pixelSize: 16; wrapMode: Text.Wrap; width: parent.width }

        Row {
          spacing: 12
          width: parent.width
          Repeater {
            model: {
              var m = [
                { l: "Deny", k: "D", c: "#4a2020", decision: "deny" },
                { l: "Approve Once", k: "O", c: "#2a2a2a", decision: "allow-once" }
              ]
              if (root.offerAllowSession) m.push({ l: "Allow for Session", k: "S", c: "#204a2e", decision: "allow-session" })
              return m
            }
            delegate: Rectangle {
              width: root.offerAllowSession ? 168 : 258; height: 46; radius: 9
              color: modelData.c
              border.color: "#666"; border.width: 1
              Text { anchors.centerIn: parent; text: "[" + modelData.k + "] " + modelData.l; color: "white"; font.pixelSize: 14 }
              MouseArea { anchors.fill: parent; onClicked: root.respond(modelData.decision) }
            }
          }
        }
      }
    }
  }
}
