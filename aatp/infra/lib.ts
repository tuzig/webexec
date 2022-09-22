export function getOffer(page): Promise<string> {
    return page.evaluate(() => {
        window.candidates = []
        return new Promise<{pc, offer}>((resolve, reject) => {
            window.connectionState = "init"
            window.pc = new RTCPeerConnection({
              iceServers: [{
                urls: 'stun:stun.l.google.com:19302',
              }],
            })
            const sendChannel = pc.createDataChannel('%')
            sendChannel.onclose = () => console.log('cdcChannel has closed')
            sendChannel.onopen = () => console.log('cdcChannel has opened')

            pc.onconnectionstatechange = ev => 
                window.connectionState =  ev.connectionState
            pc.onnegotiationneeded = () => {
                pc.createOffer().then(offer => {
                    const str = JSON.stringify(offer)
                    console.log("offer", str)
                    pc.setLocalDescription(offer)
                    resolve(str)
                }).catch(e => reject(e))
            }
            pc.onicecandidate = event => window.candidates.push(event.candidate)
        })
    })
}
