import { test, expect, Page, BrowserContext } from '@playwright/test'
import { Client } from 'ssh2'
import * as fs from 'fs'
import waitPort from 'wait-port'



const local = process.env.LOCALDEV !== undefined,
      url = local?"http://localhost:3000":"http://terminal7"

test.describe('use accept 7session', ()  => {

    const sleep = (ms) => { return new Promise(r => setTimeout(r, ms)) }

    let page: Page,
        context: BrowserContext

    test.beforeAll(async ({ browser }) => {
        context = await browser.newContext()
        page = await context.newPage()
        page.on('console', (msg) => console.log('console log:', msg.text()))
        page.on('pageerror', (err: Error) => console.log('PAGEERROR', err.message))
        await waitPort({host:'client', port:80})
        const response = await page.goto(url)
        await expect(response.ok(), `got error ${response.status()}`).toBeTruthy()
        // first page session for just for storing the dotfiles
        await waitPort({host:'webexec', port:7777})
    })

    test('use accept to connect to gate', async () => {
        
        const conn = new Client();
        conn.on('ready', () => {
          conn.shell((err, stream) => {
            if (err) throw err;
            stream.on('close', (code, signal) => {
              console.log('Stream :: close :: code: ' + code + ', signal: ' + signal);
              conn.end();
            }).on('data', (data) => {
              console.log('STDOUT: ' + data);
            }).stderr.on('data', (data) => {
              console.log('STDERR: ' + data);
            });
          });
        }).connect({
          host: 'webexec',
          port: 22,
          username: 'webexec',
          password: 'webexec'
        });

    })
})
