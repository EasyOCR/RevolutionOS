import React from 'react'
import {createRoot} from 'react-dom/client'
import '@mantine/core/styles.css'
import {MantineProvider, createTheme} from '@mantine/core'
import './style.css'
import App from './App'

// Matches ui/frontend and panel/frontend's light theme exactly, so the login
// screen, panel, and orchestrator dashboard read as one product.
const theme = createTheme({
    primaryColor: 'blue',
})

const container = document.getElementById('root')

const root = createRoot(container!)

root.render(
    <React.StrictMode>
        <MantineProvider theme={theme} defaultColorScheme="light">
            <App/>
        </MantineProvider>
    </React.StrictMode>
)
