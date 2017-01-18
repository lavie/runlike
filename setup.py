from setuptools import setup

setup(
    name='runlike',
    version='0.1.4',
    py_modules=['runlike'],
    description='Reverse-engineer docker run command line arguments based on running containers',
    author='Assaf Lavie',
    author_email='a@assaflavie.com',
    url='https://github.com/assaflavie/runlike/',
    download_url = 'https://github.com/assaflavie/runlike/tarball/0.1.2',
    keywords=['docker', 'cli'],
    install_requires=[
        'Click',
    ],
    entry_points='''
        [console_scripts]
        runlike=runlike:cli
    '''
)
