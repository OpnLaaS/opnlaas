/**
 * This is the configuration for webpack which handles bundling our js
 * The setup here is pretty non-standard since were using ecmascript instead of commonjs
 */

import TerserPlugin from "terser-webpack-plugin";

const __dirname = import.meta.dirname;

export default function (env) {
    return {
        optimization: {
            minimize: true,
            minimizer: [
                new TerserPlugin({
                    test: /\.js$/i,
                    parallel: true,
                    terserOptions: {
                        mangle: true,
                    },
                }),
            ],
        },


        // entry specifies a js bundle that will be created in dist
        // these can be accessed in html files
        entry: {
            global: "./public/scripts/global.js",
            dashboard: "./public/scripts/dashboard.js",
            hosts: "./public/scripts/hosts.js"
        },

        // all files are output into /public/dist
        output: {
            filename: "[name].js",
            path: `${__dirname}/public/static/dist/scripts`,
            clean: true,
        },
    };
};