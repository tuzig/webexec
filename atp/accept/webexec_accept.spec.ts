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
        // await waitPort({host:'webexec', port:7777})
        const response = await page.goto("http://client")
        await expect(response.ok()).toBeTruthy()
        // first page session for just for storing the dotfiles
    })

    test('it can accept an offer and candidates', async () => {
        const conn = new Client()
        let cmdClosed = false
        console.log("dfdf")
        conn.on('error', e => console.log("ssh error", e))
        conn.on('close', e => console.log("ssh closed", e))
        conn.on('end', e => console.log("ssh ended", e))
        conn.on('keyboard-interactive', e => console.log("ssh interaction", e))
        conn.on('ready', () => {
          conn.shell(async (err, stream) => {
            if (err) throw err;
            console.log("got shell")
            let { pc, offer } = await page.evaluate(async () => {
                window.candidates = []
                return new Promise<{pc, offer}>((resolve) => {
                    pc = new RTCPeerConnection({
                      iceServers: [{
                        urls: 'stun:stun.l.google.com:19302',
                      }],
                    })
                    console.log(2)
                    const sendChannel = pc.createDataChannel('%')
                    sendChannel.onclose = () => console.log('cdcChannel has closed')
                    sendChannel.onopen = () => console.log('cdcChannel has opened')
                    console.log(3)

                    pc.onconnectionstatechange = ev => 
                        window.connectionState =  ev.connectionState
                    pc.onnegotiationneeded = () => {
                        pc.createOffer().then(offer => {
                              pc.setLocalDescription(offer)
                              resolve({ pc, offer })
                        }).catch(e => reject(e))
                    }
                    console.log(4)
                    pc.onicecandidate = event => window.candidates.push(event.candidate)
                })
            })
            stream.on('close', (code, signal) => {
              expect(signal).toEqual(0)
              cmdClosed = true
              conn.end()
            }).on('data', async (data) => {
              let s = JSON.parse(data)
              console.log("adding candidate", s)
              await page.evaluate(can => pc.addIceCandidate(can), s)
            }).stderr.on('data', (data) => {
              console.log("ERROR: " + data)
            })
            while(!cmdClosed) {
                sleep(100)
                candidates = await page.evaluate(() => {
                    let ret = candidates
                    candidates = []
                    return ret
                })
                candidates.forEach(c => stream.write(c))
            }
          })
        }).connect({
          host: 'webexec',
          port: 22,
          username: 'webexec',
          password: 'webexec'
        })
        while(!cmdClosed)
            sleep(200)
    })
})
