import { test, expect, Page, BrowserContext } from '@playwright/test'
import { Client } from 'ssh2'
import { getOffer } from '../infra/lib'
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
                  username: 'runner',
                  password: 'webexec'
                })
            })
        } catch(e) { expect(e).toBeNull() }
        // log key SSH events
        conn.on('error', e => console.log("ssh error", e))
        conn.on('close', e => {
            cmdClosed = true
            console.log("ssh closed", e)
        })
        conn.on('end', e => console.log("ssh ended", e))
        conn.on('keyboard-interactive', e => console.log("ssh interaction", e))
        try {
            stream = await new Promise((resolve, reject) => {
                conn.exec("webexec accept", { pty: true }, async (err, s) => {
                    if (err)
                        reject(err)
                    else 
                        resolve(s)
                })
            })
        } catch(e) { expect(e).toBeNull() }
        let dataLines = 0
        stream.on('close', (code, signal) => {
            console.log(`closed with ${signal}`)
            cmdClosed = true
            conn.end()
        }).on('data', async (data) => {
            const line = "" + data
            dataLines++
            console.log(`>${dataLines} ${line}`)
            let s
            try {
                s = JSON.parse(data)
                console.log("parssed succesfully")
            } catch(e) { return }
            await page.evaluate(can => window.pc.addIceCandidate(can), s)
        }).stderr.on('data', (data) => {
              console.log("ERROR: " + data)
        })
        const offer = await getOffer(page)
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
                pcState = await page.evaluate(() => window.connectionState)
            } catch(e) { expect(e).toBeNull() }
            console.log("PC state", pcState)
            await sleep(500)
        }
    })
})
