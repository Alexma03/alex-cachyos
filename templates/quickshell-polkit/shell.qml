// Minimal Omarchy-style polkit agent for Niri.
// Fingerprint: square + icon only. Password: lock + field. Esc cancels. No Cancel button.
import QtQuick
import Quickshell
import Quickshell.Io
import Quickshell.Services.Polkit
import Quickshell.Wayland
import "PolkitModel.js" as PolkitModel

Scope {
  id: root

  // Catppuccin Mocha-ish, matches Noctalia dark look
  readonly property color accent: "#89b4fa"
  readonly property color background: "#1e1e2e"
  readonly property color foreground: "#cdd6f4"
  readonly property color textMuted: "#6c7086"
  readonly property color textError: "#f38ba8"
  readonly property color border: "#313244"
  // Very light glass tint — blur does the rest.
  readonly property color scrim: "#5511111b"
  readonly property color glassBg: "#cc1e1e2e"
  readonly property string fontFamily: "MesloLGS Nerd Font"
  readonly property int cornerRadius: 14
  readonly property int fieldHeight: 44
  readonly property int contentMargin: 14

  property bool closing: false
  property bool submitted: false
  property string currentMessage: ""
  property bool responseRequired: false
  property bool responseVisible: false
  property bool errorFlash: false
  property bool fingerprintConfigured: false
  property int shakeOffset: 0

  readonly property bool dialogVisible: polkitAgent.isActive || closing
  readonly property bool fingerprintMode: fingerprintConfigured && dialogVisible && !responseRequired && !submitted && !errorFlash
  readonly property int cardHeight: fieldHeight + contentMargin * 2
  readonly property int cardWidth: fingerprintMode ? cardHeight : 300

  function resetSnapshot() {
    currentMessage = ""
    responseRequired = false
    responseVisible = false
    errorFlash = false
    submitted = false
    passwordInput.text = ""
  }

  function syncFromFlow() {
    var flow = polkitAgent.flow
    if (!flow) return
    currentMessage = String(flow.message || "")
    responseRequired = !!flow.isResponseRequired
    responseVisible = !!flow.responseVisible
    if (responseRequired) submitted = false
  }

  function beginFlow() {
    closeTimer.stop()
    closing = false
    submitted = false
    passwordInput.text = ""
    syncFromFlow()
    Qt.callLater(refocus)
  }

  function refocus() {
    if (!dialogVisible) return
    if (fingerprintMode) keyCatcher.forceActiveFocus()
    else passwordInput.forceActiveFocus()
  }

  function submitResponse() {
    var flow = polkitAgent.flow
    if (!flow || !flow.isResponseRequired) return
    submitted = true
    errorFlash = false
    flow.submit(passwordInput.text)
    passwordInput.text = ""
    keyCatcher.forceActiveFocus()
  }

  function cancelRequest() {
    var flow = polkitAgent.flow
    passwordInput.text = ""
    submitted = false
    closing = true
    closeTimer.restart()
    if (flow) flow.cancelAuthenticationRequest()
  }

  function triggerFailureFeedback() {
    submitted = false
    errorFlash = true
    passwordInput.text = ""
    errorTimer.restart()
    shakeAnimation.restart()
    Qt.callLater(refocus)
  }

  Timer {
    id: closeTimer
    interval: 220
    onTriggered: {
      closing = false
      resetSnapshot()
    }
  }

  Timer {
    id: errorTimer
    interval: 1100
    onTriggered: root.errorFlash = false
  }

  SequentialAnimation {
    id: shakeAnimation
    NumberAnimation { target: root; property: "shakeOffset"; to: -8; duration: 35 }
    NumberAnimation { target: root; property: "shakeOffset"; to: 8; duration: 50 }
    NumberAnimation { target: root; property: "shakeOffset"; to: 0; duration: 55 }
  }

  FileView {
    path: "/etc/pam.d/polkit-1"
    watchChanges: true
    printErrors: false
    onLoaded: root.fingerprintConfigured = PolkitModel.fingerprintConfiguredFromPamConfig(text())
    onLoadFailed: root.fingerprintConfigured = false
    onFileChanged: reload()
  }

  PolkitAgent {
    id: polkitAgent
    path: "/org/alexcachyos/PolkitAgent"

    onAuthenticationRequestStarted: root.beginFlow()
    onIsActiveChanged: {
      if (isActive) root.syncFromFlow()
      else if (!root.closing) root.resetSnapshot()
    }
  }

  Connections {
    target: polkitAgent.flow
    function onIsResponseRequiredChanged() {
      root.syncFromFlow()
      if (!polkitAgent.flow || !polkitAgent.flow.isResponseRequired) passwordInput.text = ""
      Qt.callLater(root.refocus)
    }
    function onResponseVisibleChanged() { root.syncFromFlow() }
    function onAuthenticationFailed() {
      root.syncFromFlow()
      root.triggerFailureFeedback()
    }
    function onAuthenticationSucceeded() {
      root.closing = true
      closeTimer.restart()
    }
    function onAuthenticationRequestCancelled() {
      root.closing = true
      closeTimer.restart()
    }
  }

  PanelWindow {
    id: panel
    visible: root.dialogVisible
    anchors { top: true; bottom: true; left: true; right: true }
    color: "transparent"
    WlrLayershell.namespace: "alex-cachyos-polkit"
    WlrLayershell.layer: WlrLayer.Overlay
    WlrLayershell.keyboardFocus: WlrKeyboardFocus.Exclusive
    exclusionMode: ExclusionMode.Ignore

    // Blur desktop behind the modal (niri ext-background-effect).
    BackgroundEffect.blurRegion: Region { item: panel.contentItem }

    Rectangle {
      anchors.fill: parent
      color: root.scrim
    }

    MouseArea {
      anchors.fill: parent
      onClicked: root.refocus()
    }

    Rectangle {
      width: Math.min(justificationText.implicitWidth + 24, panel.width - 48)
      height: 28
      anchors.horizontalCenter: card.horizontalCenter
      anchors.bottom: card.top
      anchors.bottomMargin: 10
      radius: root.cornerRadius
      color: root.glassBg
      border.width: 0
      visible: root.dialogVisible && justificationText.text.length > 0

      Text {
        id: justificationText
        anchors.fill: parent
        anchors.leftMargin: 12
        anchors.rightMargin: 12
        text: PolkitModel.authorizationLabel(root.currentMessage)
        color: root.foreground
        font.family: root.fontFamily
        font.pixelSize: 12
        horizontalAlignment: Text.AlignHCenter
        verticalAlignment: Text.AlignVCenter
        elide: Text.ElideMiddle
      }
    }

    Rectangle {
      id: card
      width: root.cardWidth
      height: root.cardHeight
      radius: root.cornerRadius
      anchors.centerIn: parent
      anchors.horizontalCenterOffset: root.shakeOffset
      color: root.glassBg
      border.width: 0

      MouseArea {
        anchors.fill: parent
        onClicked: root.refocus()
      }

      Item {
        id: keyCatcher
        anchors.fill: parent
        focus: true
        Keys.priority: Keys.BeforeItem
        Keys.onPressed: function(event) {
          if (event.key === Qt.Key_Escape) {
            root.cancelRequest()
            event.accepted = true
          } else if (event.key === Qt.Key_Return || event.key === Qt.Key_Enter) {
            if (root.responseRequired) root.submitResponse()
            event.accepted = true
          }
        }
      }

      // Fingerprint: icon alone (Omarchy biometric card). Nerd Font md-fingerprint.
      Text {
        anchors.centerIn: parent
        visible: root.fingerprintMode
        text: "\udb80\ude37"
        color: root.errorFlash ? root.textError : root.accent
        font.family: root.fontFamily
        font.pixelSize: Math.round(root.fieldHeight * 0.7)
      }

      // Password: lock + field, no buttons
      Row {
        visible: !root.fingerprintMode
        anchors.fill: parent
        anchors.margins: root.contentMargin
        spacing: 14

        Text {
          text: "\uf023"
          color: root.errorFlash ? root.textError : root.accent
          font.family: root.fontFamily
          font.pixelSize: 22
          width: 26
          height: root.fieldHeight
          horizontalAlignment: Text.AlignHCenter
          verticalAlignment: Text.AlignVCenter
        }

        Item {
          width: parent.width - 40
          height: root.fieldHeight

          TextInput {
            id: passwordInput
            anchors.fill: parent
            verticalAlignment: TextInput.AlignVCenter
            clip: true
            font.family: root.fontFamily
            font.pixelSize: 16
            echoMode: root.responseVisible ? TextInput.Normal : TextInput.Password
            passwordCharacter: "\u2022"
            color: root.errorFlash ? root.textError : root.foreground
            cursorVisible: activeFocus && !root.submitted && !root.errorFlash
            readOnly: root.submitted || root.errorFlash
            enabled: root.dialogVisible
            onAccepted: root.submitResponse()
            Keys.onPressed: function(event) {
              if (event.key === Qt.Key_Escape) {
                root.cancelRequest()
                event.accepted = true
              }
            }
          }

          Text {
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.verticalCenter: parent.verticalCenter
            text: root.errorFlash ? "Wrong" : (root.submitted ? "Checking\u2026" : "Enter password")
            color: root.errorFlash ? root.textError : root.textMuted
            font.family: root.fontFamily
            font.pixelSize: 16
            elide: Text.ElideRight
            visible: passwordInput.text.length === 0
          }

          MouseArea {
            anchors.fill: parent
            onClicked: passwordInput.forceActiveFocus()
          }
        }
      }
    }
  }
}
