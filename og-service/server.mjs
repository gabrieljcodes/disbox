import express from 'express';
import satori from 'satori';
import { html } from 'satori-html';
import { Resvg } from '@resvg/resvg-js';
import fs from 'fs';
import path from 'path';
import { fileURLToPath } from 'url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

const app = express();
const port = process.env.PORT || 3000;

let interRegular;
let interBold;
let templateCache = '';
let logoBase64 = '';

async function init() {
    console.log("Downloading fonts...");
    const regRes = await fetch('https://og-playground.vercel.app/inter-latin-ext-400-normal.woff');
    interRegular = await regRes.arrayBuffer();
    
    const boldRes = await fetch('https://og-playground.vercel.app/inter-latin-ext-700-normal.woff');
    interBold = await boldRes.arrayBuffer();

    console.log("Loading template...");
    templateCache = fs.readFileSync(path.join(__dirname, 'og-template.html'), 'utf8');

    console.log("Loading logo...");
    try {
        const logoData = fs.readFileSync(path.join(__dirname, 'logo-og.png'));
        logoBase64 = logoData.toString('base64');
    } catch (e) {
        console.error("Failed to load logo-og.png", e);
    }
}

app.get('/generate', async (req, res) => {
    try {
        const { name = 'Unknown File', size = '0 B', hash = 'N/A', type = 'unknown' } = req.query;

        let finalHtml = templateCache
            .replace(/{{NAME}}/g, name)
            .replace(/{{SIZE}}/g, size)
            .replace(/{{HASH}}/g, hash)
            .replace(/{{TYPE}}/g, type)
            .replace(/{{LOGO_B64}}/g, logoBase64);

        const markup = html(finalHtml);

        const svg = await satori(markup, {
            width: 1200,
            height: 630,
            fonts: [
                {
                    name: 'Inter',
                    data: interRegular,
                    weight: 400,
                    style: 'normal',
                },
                {
                    name: 'Inter',
                    data: interBold,
                    weight: 700,
                    style: 'normal',
                }
            ],
        });

        const resvg = new Resvg(svg, {
            fitTo: { mode: 'width', value: 1200 }
        });
        const pngData = resvg.render();
        const pngBuffer = pngData.asPng();

        res.setHeader('Content-Type', 'image/png');
        res.setHeader('Cache-Control', 'public, max-age=31536000, immutable');
        res.send(pngBuffer);
    } catch (error) {
        console.error("Error generating OG image:", error);
        res.status(500).send('Internal Server Error');
    }
});

init().then(() => {
    app.listen(port, () => {
        console.log(`OG Service listening at http://localhost:${port}`);
    });
}).catch(console.error);
