import { test, expect, Page, BrowserContext } from '@playwright/test'
import { Client } from 'ssh2'
import * as fs from 'fs'
import waitPort from 'wait-port'

test.describe('use webexec accept to start a session', ()  => {

    const sleep = (ms) => { return new Promise(r => setTimeout(r, ms)) }

    let page: Page,
        context: BrowserContext

    test.beforeAll(async ({ browser }) => {
        context = await browser.newContext()
        page = await context.newPage()
        page.on('console', (msg) => console.log('console log:', msg.text()))
        page.on('pageerror', (err: Error) => console.log('PAGEERROR', err.message))
        const response = await page.goto("http://client")
        await expect(response.ok()).toBeTruthy()
        await waitPort({host:'webexec', port:7777})
    })

    test('it can accept an offer and candidates', async () => {
        let cmdClosed = false
        let conn, stream
        try {
            conn = await new Promise((resolve, reject) => {
                const conn = new Client()
                conn.on('error', e => reject(e))
                conn.on('ready', () => resolve(conn))
                conn.connect({
                  host: 'webexec',
                  port: 22,
                  username: 'webexec',
                  password: 'webexec'
                })
            })
        } catch(e) { expect(e).toBeNull() }
        try {
            stream = await new Promise((resolve, reject) => {
                conn.shell(async (err, s) => {
                    if (err)
                        reject(err)
                    else 
                        resolve(s)
                })
            })
        } catch(e) { expect(e).toBeNull() }
        let dataLines = 0
        stream.on('close', (code, signal) => {
          cmdClosed = true
          conn.end()
        }).on('data', async (data) => {
            console.log("data: " + data)
            /*
            dataLines++
            if (dataLines < 3)
                return
            else if (dataLines == 2) {
                expect(""+data).toEqual("$ ")
            }*/

          // let s = JSON.parse(data)
          // await page.evaluate(can => windows.pc.addIceCandidate(can), s)
        }).stderr.on('data', (data) => {
          console.log("ERROR: " + data)
        })
        conn.on('error', e => console.log("ssh error", e))
        conn.on('close', e => {
            cmdClosed = true
            console.log("ssh closed", e)
        })
        conn.on('end', e => console.log("ssh ended", e))
        conn.on('keyboard-interactive', e => console.log("ssh interaction", e))
        let offer
        try {
            offer = await page.evaluate(async () => {
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
                            console.log("offer", offer)
                            pc.setLocalDescription(offer)
                            resolve(JSON.stringify(offer))
                        }).catch(e => reject(e))
                    }
                    pc.onicecandidate = event => window.candidates.push(event.candidate)
                })
            })
        } catch(e) { expect(e).toBeNull() }
        stream.write("./webexec accept\n")
        stream.write(offer + "\n")
        let pcState = null
        while (pcState != "connected") {
            let cans = []
            try {
                cans = await page.evaluate(() => {
                    ret = window.candidates
                    window.candidates = []
                    return ret
                })
            } catch(e) { expect(e).toBeNull() }
            cans.forEach((c) => stream.write(JSON.stringify(c)+"\n"))
            try {
                let pcState = await page.evaluate(() => window.connectionState)
            } catch(e) { expect(e).toBeNull() }
            console.log("PC state", pcState)
            await sleep(500)
        }
    })
})
